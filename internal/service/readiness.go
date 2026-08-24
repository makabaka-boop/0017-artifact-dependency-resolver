package service

import (
	"artifact-dep-resolver/internal/errcode"
	"artifact-dep-resolver/internal/model"
	"artifact-dep-resolver/internal/resolver"
)

// ReadinessReport 是发布就绪检查的结果。
type ReadinessReport struct {
	Artifact string             `json:"artifact"`
	Version  string             `json:"version"`
	Ready    bool               `json:"ready"`
	Blockers []ReadinessBlocker `json:"blockers"`
	Resolved []ResolvedEntry    `json:"resolved,omitempty"`
}

// ReadinessBlocker 描述一条阻断发布的原因。
type ReadinessBlocker struct {
	Type    string   `json:"type"`
	Message string   `json:"message"`
	Details []string `json:"details"`
}

// CheckReadiness 校验某制品的指定版本（缺省最高版本）的全量传递依赖是否可解析，
// 并列出全部阻断项。就绪判定等价于一次成功的完整解析。
func (s *Service) CheckReadiness(name, version string) (*ReadinessReport, error) {
	if _, err := s.store.GetArtifactByName(name); err != nil {
		return nil, errcode.New(errcode.NotFound, "artifact not found")
	}
	var verRef *string
	if version != "" {
		verRef = &version
	}
	// 若未指定版本，则校验最高版本。
	graph, err := s.store.LoadGraph()
	if err != nil {
		return nil, errcode.New(errcode.Internal, "load graph failed")
	}
	ix := resolver.NewIndex(buildResolverArtifacts(graph))
	root := ix.ArtifactByName(name)
	report := &ReadinessReport{
		Artifact: name,
		Version:  version,
		Ready:    true,
		Blockers: []ReadinessBlocker{},
	}
	if root == nil || len(root.Versions) == 0 {
		report.Ready = false
		report.Blockers = append(report.Blockers, ReadinessBlocker{
			Type: "MISSING", Message: "artifact has no versions", Details: []string{name},
		})
		return report, nil
	}
	if verRef == nil {
		top := root.Versions[0].Version
		report.Version = top
		verRef = &top
	}

	res := resolver.Resolve(ix, name, verRef)
	if res.Status != model.ResolutionSucceeded {
		report.Ready = false
		for _, d := range res.Diagnostics {
			report.Blockers = append(report.Blockers, ReadinessBlocker{
				Type: d.Type, Message: d.Message, Details: d.Details,
			})
		}
		return report, nil
	}
	for _, sel := range res.Resolved {
		report.Resolved = append(report.Resolved, ResolvedEntry{Artifact: sel.ArtifactName, Version: sel.Version})
	}
	sortResolvedEntries(report.Resolved)
	return report, nil
}
