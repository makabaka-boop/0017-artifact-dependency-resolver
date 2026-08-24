package service

import (
	"encoding/json"
	"fmt"
	"time"

	"artifact-dep-resolver/internal/constraint"
	"artifact-dep-resolver/internal/errcode"
	"artifact-dep-resolver/internal/model"
	"artifact-dep-resolver/internal/resolver"
	"artifact-dep-resolver/internal/semver"
	"artifact-dep-resolver/internal/store"
)

// Service 是业务编排层。
type Service struct {
	store store.Store
	now   func() time.Time
}

// New 构造 Service。
func New(s store.Store) *Service {
	return &Service{store: s, now: time.Now}
}

func (s *Service) nowTime() time.Time { return s.now().UTC() }

// CreateArtifact 登记制品。
func (s *Service) CreateArtifact(name string) (*model.Artifact, error) {
	if !model.ValidArtifactName(name) {
		return nil, errcode.New(errcode.BadRequest, "invalid artifact name")
	}
	if _, err := s.store.GetArtifactByName(name); err == nil {
		return nil, errcode.New(errcode.Conflict, "artifact already exists")
	}
	a, err := s.store.CreateArtifact(name, s.nowTime())
	if err != nil {
		return nil, errcode.New(errcode.Internal, "create artifact failed")
	}
	if err := s.recordChange("artifact", a.ID, "create", "", mustJSON(a)); err != nil {
		return nil, err
	}
	return a, nil
}

// GetArtifact 按名查询制品。
func (s *Service) GetArtifact(name string) (*model.Artifact, error) {
	a, err := s.store.GetArtifactByName(name)
	if err != nil {
		return nil, errcode.New(errcode.NotFound, "artifact not found")
	}
	return a, nil
}

// ListArtifacts 分页查询制品。
func (s *Service) ListArtifacts(limit, offset int) ([]model.Artifact, error) {
	return s.store.ListArtifacts(limit, offset)
}

// CreateVersion 登记版本。
func (s *Service) CreateVersion(name, version string) (*model.ArtifactVersion, error) {
	a, err := s.store.GetArtifactByName(name)
	if err != nil {
		return nil, errcode.New(errcode.NotFound, "artifact not found")
	}
	v, err := semver.Parse(version)
	if err != nil {
		return nil, errcode.New(errcode.BadRequest, "invalid semver")
	}
	if _, err := s.store.GetVersion(a.ID, v.String()); err == nil {
		return nil, errcode.New(errcode.Conflict, "version already exists")
	}
	created, err := s.store.CreateVersion(a.ID, v.String(), s.nowTime())
	if err != nil {
		return nil, errcode.New(errcode.Internal, "create version failed")
	}
	if err := s.recordChange("artifact_version", created.ID, "create", "", mustJSON(created)); err != nil {
		return nil, err
	}
	return created, nil
}

// GetVersion 查版本详情。
func (s *Service) GetVersion(name, version string) (*model.ArtifactVersion, error) {
	a, err := s.store.GetArtifactByName(name)
	if err != nil {
		return nil, errcode.New(errcode.NotFound, "artifact not found")
	}
	v, err := s.store.GetVersion(a.ID, version)
	if err != nil {
		return nil, errcode.New(errcode.NotFound, "version not found")
	}
	return v, nil
}

// ListVersions 列出某制品的版本。
func (s *Service) ListVersions(name string) ([]model.ArtifactVersion, error) {
	a, err := s.store.GetArtifactByName(name)
	if err != nil {
		return nil, errcode.New(errcode.NotFound, "artifact not found")
	}
	return s.store.ListVersions(a.ID)
}

// DependencyInput 是依赖声明的输入结构。
type DependencyInput struct {
	Artifact   string `json:"artifact"`
	Constraint string `json:"constraint"`
}

// ReplaceDependencies 全量替换某版本的依赖。
func (s *Service) ReplaceDependencies(name, version string, inputs []DependencyInput) error {
	a, err := s.store.GetArtifactByName(name)
	if err != nil {
		return errcode.New(errcode.NotFound, "artifact not found")
	}
	v, err := s.store.GetVersion(a.ID, version)
	if err != nil {
		return errcode.New(errcode.NotFound, "version not found")
	}
	before, _ := s.store.ListDependencies(v.ID)

	var deps []model.Dependency
	seen := map[int64]bool{}
	for _, in := range inputs {
		if _, err := constraint.Parse(in.Constraint); err != nil {
			return errcode.New(errcode.BadRequest, "invalid constraint")
		}
		ta, err := s.store.GetArtifactByName(in.Artifact)
		if err != nil {
			return errcode.New(errcode.Unprocessable, "dependency target not found")
		}
		if seen[ta.ID] {
			return errcode.New(errcode.BadRequest, "duplicate dependency target")
		}
		seen[ta.ID] = true
		deps = append(deps, model.Dependency{FromVersionID: v.ID, ToArtifactID: ta.ID, Constraint: in.Constraint})
	}
	if err := s.store.ReplaceDependencies(v.ID, deps, s.nowTime()); err != nil {
		return errcode.New(errcode.Internal, "replace dependencies failed")
	}
	after, _ := s.store.ListDependencies(v.ID)
	if err := s.recordChange("artifact_version", v.ID, "replace_dependencies", mustJSON(depsJSON(before)), mustJSON(depsJSON(after))); err != nil {
		return err
	}
	return nil
}

type depJSON struct {
	Constraint   string `json:"constraint"`
	ToArtifactID int64  `json:"to_artifact_id"`
}

func depsJSON(deps []model.Dependency) []depJSON {
	out := make([]depJSON, 0, len(deps))
	for _, d := range deps {
		out = append(out, depJSON{Constraint: d.Constraint, ToArtifactID: d.ToArtifactID})
	}
	return out
}

// Resolve 执行依赖解析。
func (s *Service) Resolve(rootName string, rootVersion *string) (*ResolutionOutput, error) {
	out, err := s.resolveForSource(rootName, rootVersion, model.SourceDirect, nil)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// resolveForSource 是解析的共享实现：以来源与（可选的）源解析标记生成快照。
func (s *Service) resolveForSource(rootName string, rootVersion *string, source string, sourceResolutionID *int64) (*ResolutionOutput, error) {
	root, err := s.store.GetArtifactByName(rootName)
	if err != nil {
		return nil, errcode.New(errcode.NotFound, "root artifact not found")
	}
	graph, err := s.store.LoadGraph()
	if err != nil {
		return nil, errcode.New(errcode.Internal, "load graph failed")
	}
	rarts := buildResolverArtifacts(graph)
	ix := resolver.NewIndex(rarts)
	res := resolver.Resolve(ix, rootName, rootVersion)

	requestRef := fmt.Sprintf("%s-%d", source, s.nextRefSeq())
	inputJSON := mustJSON(map[string]interface{}{"artifact": rootName, "version": rootVersion})

	r := &model.Resolution{
		RequestRef:         requestRef,
		RootArtifactID:     root.ID,
		InputJSON:          inputJSON,
		Status:             res.Status,
		Source:             source,
		SourceResolutionID: sourceResolutionID,
	}
	if rootVersion != nil {
		if v, err := s.store.GetVersion(root.ID, *rootVersion); err == nil {
			r.RootVersionID = &v.ID
		}
	}
	if res.Status == model.ResolutionSucceeded {
		r.ResultJSON = mustJSON(res.Resolved)
	} else {
		r.ErrorCode = "resolution_failed"
		for _, d := range res.Diagnostics {
			r.ErrorMessage = d.Message
			break
		}
		r.ResultJSON = "[]"
	}
	var items []model.ResolutionItem
	for _, sel := range res.Resolved {
		items = append(items, model.ResolutionItem{
			ArtifactID:      sel.ArtifactID,
			VersionID:       sel.VersionID,
			SelectedVersion: sel.Version,
			Depth:           sel.Depth,
			Reason:          sel.Reason,
		})
	}
	if err := s.store.SaveResolution(r, items, s.nowTime()); err != nil {
		return nil, errcode.New(errcode.Internal, "save resolution failed")
	}
	if err := s.recordChange("resolution", r.ID, "resolve", "", mustJSON(r)); err != nil {
		return nil, err
	}

	out := &ResolutionOutput{
		ResolutionID: r.ID,
		Status:       res.Status,
	}
	for _, sel := range res.Resolved {
		out.Resolved = append(out.Resolved, ResolvedEntry{Artifact: sel.ArtifactName, Version: sel.Version})
	}
	for _, d := range res.Diagnostics {
		diag := Diag{Type: d.Type, Message: d.Message, Details: d.Details}
		for _, c := range d.Candidates {
			diag.Candidates = append(diag.Candidates, DiagCandidate{
				Artifact: c.Artifact, Version: c.Version, Accepted: c.Accepted, Reason: c.Reason,
			})
		}
		out.Diagnostics = append(out.Diagnostics, diag)
	}
	return out, nil
}

// ResolvedEntry 是解析输出中的版本条目。
type ResolvedEntry struct {
	Artifact string `json:"artifact"`
	Version  string `json:"version"`
}

// DiagCandidate 是诊断中的一个候选版本及淘汰/接受原因。
type DiagCandidate struct {
	Artifact string `json:"artifact"`
	Version  string `json:"version"`
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason"`
}

// Diag 是解析诊断。
type Diag struct {
	Type       string          `json:"type"`
	Message    string          `json:"message"`
	Details    []string        `json:"details"`
	Candidates []DiagCandidate `json:"candidates,omitempty"`
}

// ResolutionOutput 是解析 API 响应。
type ResolutionOutput struct {
	ResolutionID int64           `json:"resolution_id"`
	Status       string          `json:"status"`
	Resolved     []ResolvedEntry `json:"resolved"`
	Diagnostics  []Diag          `json:"diagnostics"`
}

func (s *Service) nextRefSeq() int64 {
	n, _ := s.store.NextID()
	return n
}

func buildResolverArtifacts(graph []store.GraphData) []resolver.Artifact {
	out := make([]resolver.Artifact, 0, len(graph))
	for _, gd := range graph {
		a := resolver.Artifact{
			ID:       gd.Artifact.ID,
			Name:     gd.Artifact.Name,
			Versions: make([]resolver.ArtifactVersion, 0, len(gd.Versions)),
		}
		depWork := make([]resolver.Dep, 0, 1)
		for _, vd := range gd.Versions {
			av := resolver.ArtifactVersion{ID: vd.Version.ID, Version: vd.Version.Version}
			if v, err := semver.Parse(vd.Version.Version); err == nil {
				av.Ver = v
			}
			depWork = resolverDependencies(vd.Deps, depWork)
			av.Dependencies = depWork
			a.Versions = append(a.Versions, av)
		}
		out = append(out, a)
	}
	return out
}

func resolverDependencies(input []store.DepData, depWork []resolver.Dep) []resolver.Dep {
	if cap(depWork) == 0 {
		depWork = make([]resolver.Dep, 0, 1)
	}
	depWork = depWork[:0]
	for _, d := range input {
		c, err := constraint.Parse(d.Constraint)
		if err != nil {
			continue
		}
		depWork = append(depWork, resolver.Dep{
			ToArtifactID: d.ToArtifactID,
			ToName:       d.ToName,
			Constraint:   c,
		})
	}
	return depWork
}

// rollback 相关实现在回滚文件。
func (s *Service) recordChange(entityType string, entityID int64, action, before, after string) error {
	return s.store.AddChange(&model.ChangeRecord{EntityType: entityType, EntityID: entityID, Action: action, BeforeJSON: before, AfterJSON: after}, s.nowTime())
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
