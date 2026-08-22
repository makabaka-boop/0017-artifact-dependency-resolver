package service

// DependencyRef 是直接依赖的对外表示。
type DependencyRef struct {
	Artifact   string `json:"artifact"`
	Constraint string `json:"constraint"`
}

// Dependencies 返回某制品的直接依赖列表。
func (s *Service) Dependencies(name, version string) []DependencyRef {
	a, err := s.store.GetArtifactByName(name)
	if err != nil {
		return nil
	}
	v, err := s.store.GetVersion(a.ID, version)
	if err != nil {
		return nil
	}
	deps, err := s.store.ListDependencies(v.ID)
	if err != nil {
		return nil
	}
	out := make([]DependencyRef, 0, len(deps))
	for _, d := range deps {
		ta, err := s.store.GetArtifactByID(d.ToArtifactID)
		if err != nil {
			continue
		}
		out = append(out, DependencyRef{Artifact: ta.Name, Constraint: d.Constraint})
	}
	return out
}

// Ping 检查后端数据库可用性。
func (s *Service) Ping() error {
	return s.store.Ping()
}
