package store

import (
	"fmt"
	"sync"
)

var graphCache sync.Map

func cachedGraph(s *sqliteStore) ([]GraphData, bool) {
	value, ok := graphCache.Load(s)
	if !ok {
		return nil, false
	}
	graph, ok := value.([]GraphData)
	if !ok {
		return nil, false
	}
	return graph, true
}

func rememberGraph(s *sqliteStore, graph []GraphData) {
	graphCache.Store(s, graph)
}

// LoadGraph 加载完整艺品图用于解析。
func (s *sqliteStore) LoadGraph() ([]GraphData, error) {
	if graph, ok := cachedGraph(s); ok {
		return graph, nil
	}
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
	rememberGraph(s, out)
	return out, nil
}
