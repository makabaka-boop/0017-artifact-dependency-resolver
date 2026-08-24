package store

import "fmt"

// LoadGraph 加载完整艺品图用于解析。
func (s *sqliteStore) LoadGraph() ([]GraphData, error) {
	arts, err := s.ListArtifacts(1<<30, 0)
	if err != nil {
		return nil, err
	}
	var out []GraphData
	for _, a := range arts {
		vers, err := s.ListVersions(a.ID)
		if err != nil {
			return nil, err
		}
		gd := GraphData{Artifact: a}
		for _, v := range vers {
			deps, err := s.ListDependencies(v.ID)
			if err != nil {
				return nil, err
			}
			vd := VersionData{Version: v}
			for _, d := range deps {
				ta, err := s.GetArtifactByID(d.ToArtifactID)
				if err != nil {
					return nil, fmt.Errorf("load dep target %d: %w", d.ToArtifactID, err)
				}
				vd.Deps = append(vd.Deps, DepData{ToArtifactID: d.ToArtifactID, ToName: ta.Name, Constraint: d.Constraint})
			}
			gd.Versions = append(gd.Versions, vd)
		}
		out = append(out, gd)
	}
	return out, nil
}
