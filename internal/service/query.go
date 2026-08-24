package service

import (
	"strings"

	"artifact-dep-resolver/internal/errcode"
	"artifact-dep-resolver/internal/model"
	"artifact-dep-resolver/internal/resolver"
	"artifact-dep-resolver/internal/semver"
)

// DepNode 是依赖图中的一个节点。
type DepNode struct {
	Artifact string `json:"artifact"`
	Version  string `json:"version"`
	Depth    int    `json:"depth"`
}

// DepGraph 是依赖图查询结果。
type DepGraph struct {
	Artifact     string    `json:"artifact"`
	Dependencies []DepNode `json:"dependencies"`
	Cycles       []string  `json:"cycles,omitempty"`
}

// DependencyGraph 构建制品或版本的依赖图。
func (s *Service) DependencyGraph(name string, version string, depth int) (*DepGraph, error) {
	a, err := s.store.GetArtifactByName(name)
	if err != nil {
		return nil, errcode.New(errcode.NotFound, "artifact not found")
	}
	graph, err := s.store.LoadGraph()
	if err != nil {
		return nil, errcode.New(errcode.Internal, "load graph failed")
	}
	rarts := buildResolverArtifacts(graph)
	ix := resolver.NewIndex(rarts)
	root := ix.ArtifactByName(name)
	if root == nil {
		return nil, errcode.New(errcode.NotFound, "artifact not found")
	}
	// 确定起始版本：指定版本或最高版本。
	var startIdx = 0
	if version != "" {
		if v, err := semver.Parse(version); err == nil {
			for i := range root.Versions {
				if root.Versions[i].Ver.Compare(v) == 0 {
					startIdx = i
					break
				}
			}
		}
	}
	if len(root.Versions) == 0 {
		return nil, errcode.New(errcode.NotFound, "no versions")
	}
	if depth <= 0 {
		depth = 1 << 20
	}
	g := &DepGraph{Artifact: name}
	visited := map[int64]bool{a.ID: true}
	if depth >= 0 {
		collectGraph(ix, root.Versions[startIdx], 1, depth, visited, &g.Dependencies)
	}
	// 存在环时在响应中标注环路径。
	if cycle := resolver.DetectCycle(ix, a.ID); cycle != nil {
		g.Cycles = append(g.Cycles, strings.Join(cycle, " -> "))
	}
	return g, nil
}

func collectGraph(ix *resolver.Index, v resolver.ArtifactVersion, level, maxDepth int, visited map[int64]bool, out *[]DepNode) {
	if level > maxDepth {
		return
	}
	for _, d := range v.Dependencies {
		node := DepNode{Artifact: d.ToName, Depth: level}
		*out = append(*out, node)
		target := ix.ArtifactByID(d.ToArtifactID)
		if target == nil || visited[d.ToArtifactID] {
			continue
		}
		visited[d.ToArtifactID] = true
		if len(target.Versions) > 0 {
			collectGraph(ix, target.Versions[0], level+1, maxDepth, visited, out)
		}
	}
}

// ListResolutions 分页查询解析历史。
func (s *Service) ListResolutions(limit, offset int, status string) ([]model.Resolution, error) {
	return s.store.ListResolutions(limit, offset, status)
}

// GetResolution 获取解析详情。
func (s *Service) GetResolution(id int64) (*model.Resolution, []model.ResolutionItem, error) {
	r, err := s.store.GetResolution(id)
	if err != nil {
		return nil, nil, errcode.New(errcode.NotFound, "resolution not found")
	}
	items, err := s.store.ListResolutionItems(id)
	if err != nil {
		return nil, nil, errcode.New(errcode.Internal, "load resolution items failed")
	}
	return r, items, nil
}

// Rollback 基于历史解析快照创建新快照。
func (s *Service) Rollback(id int64) (*model.Resolution, error) {
	orig, err := s.store.GetResolution(id)
	if err != nil {
		return nil, errcode.New(errcode.NotFound, "resolution not found")
	}
	// 仅允许回滚成功的解析。
	if orig.Status != model.ResolutionSucceeded {
		return nil, errcode.New(errcode.Unprocessable, "cannot rollback a failed resolution")
	}
	items, err := s.store.ListResolutionItems(id)
	if err != nil {
		return nil, errcode.New(errcode.Internal, "load items failed")
	}
	newRef := "rollback-" + orig.RequestRef
	r := &model.Resolution{
		RequestRef:         newRef,
		RootArtifactID:     orig.RootArtifactID,
		RootVersionID:      orig.RootVersionID,
		InputJSON:          orig.InputJSON,
		Status:             model.ResolutionSucceeded,
		ResultJSON:         orig.ResultJSON,
		ErrorCode:          "",
		ErrorMessage:       "",
		Source:             model.SourceRollback,
		SourceResolutionID: &orig.ID,
	}
	var newItems []model.ResolutionItem
	for _, it := range items {
		newItems = append(newItems, model.ResolutionItem{
			ArtifactID:      it.ArtifactID,
			VersionID:       it.VersionID,
			SelectedVersion: it.SelectedVersion,
			Depth:           it.Depth,
			Reason:          "rollback from resolution " + itoa(orig.ID),
		})
	}
	if err := s.store.SaveResolution(r, newItems, s.nowTime()); err != nil {
		return nil, errcode.New(errcode.Internal, "save rollback failed")
	}
	if err := s.recordChange("resolution", r.ID, "rollback", mustJSON(orig), mustJSON(r)); err != nil {
		return nil, err
	}
	return r, nil
}

// ListChanges 查询变更记录。
func (s *Service) ListChanges(limit, offset int, entityType string, entityID int64) ([]model.ChangeRecord, error) {
	return s.store.ListChanges(limit, offset, entityType, entityID)
}

// GetChange 获取单条变更记录。
func (s *Service) GetChange(id int64) (*model.ChangeRecord, error) {
	c, err := s.store.GetChange(id)
	if err != nil {
		return nil, errcode.New(errcode.NotFound, "change not found")
	}
	return c, nil
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b strings.Builder
	for n > 0 {
		b.WriteByte(byte('0' + n%10))
		n /= 10
	}
	s := b.String()
	// reverse
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}
