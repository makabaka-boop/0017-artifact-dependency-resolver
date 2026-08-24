package service

import (
	"artifact-dep-resolver/internal/errcode"
)

// DependencyDiff 是同一制品两个版本的依赖差异对比结果。
type DependencyDiff struct {
	Artifact string      `json:"artifact"`
	From     string      `json:"from_version"`
	To       string      `json:"to_version"`
	Added    []DiffEntry `json:"added"`
	Removed  []DiffEntry `json:"removed"`
	Changed  []DiffEntry `json:"changed"`
}

// DiffEntry 描述一条发生变化的依赖。
type DiffEntry struct {
	Artifact      string `json:"artifact"`
	ConstraintOld string `json:"constraint_old,omitempty"`
	ConstraintNew string `json:"constraint_new,omitempty"`
}

// DiffDependencies 对比同一制品两个版本的直接依赖，输出新增、删除与约束变化三类。
func (s *Service) DiffDependencies(name, vA, vB string) (*DependencyDiff, error) {
	a, err := s.store.GetArtifactByName(name)
	if err != nil {
		return nil, errcode.New(errcode.NotFound, "artifact not found")
	}
	va, err := s.store.GetVersion(a.ID, vA)
	if err != nil {
		return nil, errcode.New(errcode.NotFound, "version "+vA+" not found")
	}
	vb, err := s.store.GetVersion(a.ID, vB)
	if err != nil {
		return nil, errcode.New(errcode.NotFound, "version "+vB+" not found")
	}

	depsA := s.dependencyMap(va.ID)
	depsB := s.dependencyMap(vb.ID)

	diff := &DependencyDiff{
		Artifact: name,
		From:     vA,
		To:       vB,
		Added:    []DiffEntry{},
		Removed:  []DiffEntry{},
		Changed:  []DiffEntry{},
	}
	// 目标制品名需要稳定的排序输出，保证确定性。
	for targetName, conB := range depsB {
		if conA, ok := depsA[targetName]; !ok {
			diff.Added = append(diff.Added, DiffEntry{Artifact: targetName, ConstraintNew: conB})
		} else if conA != conB {
			diff.Changed = append(diff.Changed, DiffEntry{Artifact: targetName, ConstraintOld: conA, ConstraintNew: conB})
		}
	}
	for targetName, conA := range depsA {
		if _, ok := depsB[targetName]; !ok {
			diff.Removed = append(diff.Removed, DiffEntry{Artifact: targetName, ConstraintOld: conA})
		}
	}
	// 排序保证跨运行输出一致。
	sortDiffEntries(diff.Added)
	sortDiffEntries(diff.Removed)
	sortDiffEntries(diff.Changed)
	return diff, nil
}

// dependencyMap 返回 {目标制品名: 约束}。
func (s *Service) dependencyMap(versionID int64) map[string]string {
	deps, err := s.store.ListDependencies(versionID)
	if err != nil {
		return map[string]string{}
	}
	m := make(map[string]string, len(deps))
	for _, d := range deps {
		ta, err := s.store.GetArtifactByID(d.ToArtifactID)
		if err != nil {
			continue
		}
		m[ta.Name] = d.Constraint
	}
	return m
}
