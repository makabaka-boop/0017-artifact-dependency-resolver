package service

import (
	"artifact-dep-resolver/internal/errcode"
	"artifact-dep-resolver/internal/model"
)

// LockfileOutput 是锁定快照的对外表示。
type LockfileOutput struct {
	ID        int64              `json:"id"`
	Name      string             `json:"name"`
	Artifacts []LockfilePinEntry `json:"artifacts"`
}

// LockfilePinEntry 是锁定快照中的一条 pin。
type LockfilePinEntry struct {
	Artifact string `json:"artifact"`
	Version  string `json:"version"`
}

// CreateLockfile 基于一次成功的解析快照生成可复现的锁定快照。
// 仅允许对 status=succeeded 的解析生成锁定，保证锁定内容可回放。
func (s *Service) CreateLockfile(resolutionID int64, name string) (*LockfileOutput, error) {
	res, err := s.store.GetResolution(resolutionID)
	if err != nil {
		return nil, errcode.New(errcode.NotFound, "resolution not found")
	}
	if res.Status != model.ResolutionSucceeded {
		return nil, errcode.New(errcode.Unprocessable, "cannot lock a failed resolution")
	}
	if name == "" {
		name = "lock-" + res.RequestRef
	}
	if _, err := s.store.GetLockfileByName(name); err == nil {
		return nil, errcode.New(errcode.Conflict, "lockfile name already exists")
	}
	items, err := s.store.ListResolutionItems(resolutionID)
	if err != nil {
		return nil, errcode.New(errcode.Internal, "load resolution items failed")
	}

	lf := &model.Lockfile{
		Name:               name,
		RootArtifactID:     res.RootArtifactID,
		RootVersionID:      res.RootVersionID,
		SourceResolutionID: res.ID,
	}
	entries := make([]model.LockfileEntry, 0, len(items))
	out := &LockfileOutput{Name: name, Artifacts: []LockfilePinEntry{}}
	for _, it := range items {
		entries = append(entries, model.LockfileEntry{
			ArtifactID:      it.ArtifactID,
			VersionID:       it.VersionID,
			SelectedVersion: it.SelectedVersion,
		})
		a, err := s.store.GetArtifactByID(it.ArtifactID)
		if err != nil {
			return nil, errcode.New(errcode.Internal, "resolve artifact name failed")
		}
		out.Artifacts = append(out.Artifacts, LockfilePinEntry{Artifact: a.Name, Version: it.SelectedVersion})
	}
	if err := s.store.SaveLockfile(lf, entries, s.nowTime()); err != nil {
		return nil, errcode.New(errcode.Internal, "save lockfile failed")
	}
	out.ID = lf.ID
	sortPinEntries(out.Artifacts)
	if err := s.recordChange("lockfile", lf.ID, "create", "", mustJSON(out)); err != nil {
		return nil, err
	}
	return out, nil
}

// GetLockfile 返回锁定快照详情（含 pin 条目）。
func (s *Service) GetLockfile(name string) (*LockfileOutput, error) {
	lf, err := s.store.GetLockfileByName(name)
	if err != nil {
		return nil, errcode.New(errcode.NotFound, "lockfile not found")
	}
	entries, err := s.store.ListLockfileEntries(lf.ID)
	if err != nil {
		return nil, errcode.New(errcode.Internal, "load lockfile entries failed")
	}
	out := &LockfileOutput{ID: lf.ID, Name: lf.Name, Artifacts: []LockfilePinEntry{}}
	for _, e := range entries {
		a, err := s.store.GetArtifactByID(e.ArtifactID)
		if err != nil {
			return nil, errcode.New(errcode.Internal, "resolve artifact name failed")
		}
		out.Artifacts = append(out.Artifacts, LockfilePinEntry{Artifact: a.Name, Version: e.SelectedVersion})
	}
	sortPinEntries(out.Artifacts)
	return out, nil
}

// ListLockfiles 分页列出锁定快照。
func (s *Service) ListLockfiles(limit, offset int) ([]model.Lockfile, error) {
	return s.store.ListLockfiles(limit, offset)
}

// ResolveWithLockfile 以锁定快照为硬性约束再次解析。
// 对每个 pin 条目，把根制品之外的制品在其解析中固定为锁定版本；
// 若新解析结果与锁定版本不一致，则以 drifted 状态返回并附带 drift 明细。
func (s *Service) ResolveWithLockfile(lockfileName string, rootName string, rootVersion *string) (*LockfileVerifyOutput, error) {
	lf, err := s.store.GetLockfileByName(lockfileName)
	if err != nil {
		return nil, errcode.New(errcode.NotFound, "lockfile not found")
	}
	entries, err := s.store.ListLockfileEntries(lf.ID)
	if err != nil {
		return nil, errcode.New(errcode.Internal, "load lockfile entries failed")
	}
	pins := map[string]string{}
	for _, e := range entries {
		a, err := s.store.GetArtifactByID(e.ArtifactID)
		if err != nil {
			continue
		}
		pins[a.Name] = e.SelectedVersion
	}

	root, ix, err := s.prepareResolutionIndex(rootName)
	if err != nil {
		return nil, err
	}
	comparison := ix.ComparisonView()
	comparison.AlignPinnedVersions(pins)
	out, err := s.executeResolution(root, ix, rootName, rootVersion, model.SourceDirect, nil)
	if err != nil {
		return nil, err
	}
	verify := &LockfileVerifyOutput{
		Lockfile:     lockfileName,
		Status:       model.LockfileInSync,
		Resolved:     out.Resolved,
		Diagnostics:  out.Diagnostics,
		Drifted:      []DriftEntry{},
		ResolutionID: out.ResolutionID,
	}
	// 用锁定版本约束校验新解析结果。
	for _, r := range out.Resolved {
		if pinned, ok := pins[r.Artifact]; ok && pinned != r.Version {
			verify.Status = model.LockfileDrifted
			verify.Drifted = append(verify.Drifted, DriftEntry{
				Artifact: r.Artifact, Locked: pinned, Resolved: r.Version,
			})
		}
	}
	return verify, nil
}

// LockfileVerifyOutput 是以锁定快照再次解析的产出。
type LockfileVerifyOutput struct {
	Lockfile     string          `json:"lockfile"`
	Status       string          `json:"status"`
	Resolved     []ResolvedEntry `json:"resolved"`
	Diagnostics  []Diag          `json:"diagnostics"`
	Drifted      []DriftEntry    `json:"drifted"`
	ResolutionID int64           `json:"resolution_id"`
}

// DriftEntry 描述锁定版本与新解析版本的漂移。
type DriftEntry struct {
	Artifact string `json:"artifact"`
	Locked   string `json:"locked"`
	Resolved string `json:"resolved"`
}
