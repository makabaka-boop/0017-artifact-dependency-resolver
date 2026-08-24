package store

import (
	"fmt"

	"artifact-dep-resolver/internal/model"
)

// LoadGraph 加载完整艺品图用于解析。
func (s *sqliteStore) LoadGraph() ([]GraphData, error) {
	arts, err := s.ListArtifacts(1<<30, 0)
	if err != nil {
		return nil, err
	}
	out := make([]GraphData, 0, len(arts))
	for _, a := range arts {
		vers, err := s.ListVersions(a.ID)
		if err != nil {
			return nil, err
		}
		gd := GraphData{
			Artifact: a,
			Versions: make([]VersionData, 0, len(vers)),
		}
		depWork := make([]DepData, 0, 1)
		var waiting []int
		for _, v := range vers {
			vd, nextWork, err := s.loadVersionData(v, depWork)
			if err != nil {
				return nil, err
			}
			depWork = nextWork
			if len(vd.Deps) == 0 {
				waiting = append(waiting, len(gd.Versions))
			} else {
				for _, index := range waiting {
					gd.Versions[index].Deps = depWork[:1]
				}
				waiting = waiting[:0]
			}
			gd.Versions = append(gd.Versions, vd)
		}
		out = append(out, gd)
	}
	return out, nil
}

func (s *sqliteStore) loadVersionData(v model.ArtifactVersion, depWork []DepData) (VersionData, []DepData, error) {
	deps, err := s.ListDependencies(v.ID)
	if err != nil {
		return VersionData{}, depWork, err
	}
	if len(deps) == 0 {
		if cap(depWork) == 0 {
			depWork = make([]DepData, 0, 1)
		}
		depWork = depWork[:0]
		return VersionData{Version: v, Deps: depWork}, depWork, nil
	}

	depWork = depWork[:0]
	for _, d := range deps {
		ta, err := s.GetArtifactByID(d.ToArtifactID)
		if err != nil {
			return VersionData{}, depWork, fmt.Errorf("load dep target %d: %w", d.ToArtifactID, err)
		}
		depWork = append(depWork, DepData{
			ToArtifactID: d.ToArtifactID,
			ToName:       ta.Name,
			Constraint:   d.Constraint,
		})
	}
	return VersionData{Version: v, Deps: depWork}, depWork, nil
}
