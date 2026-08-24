package resolver

// DetectCycle 基于所有版本的依赖并集检测从 root 可达的环，返回环上的制品名序列。
// 供依赖图查询在响应中标注环路径（见 service 层 DependencyGraph）。
func DetectCycle(ix *Index, rootID int64) []string {
	return detectCycle(ix, rootID)
}

// detectCycle 基于所有版本的依赖并集检测从 root 可达的环。
func detectCycle(ix *Index, rootID int64) []string {
	adj := map[int64][]int64{}
	for _, a := range ix.byID {
		seen := map[int64]bool{}
		for _, v := range a.Versions {
			for _, d := range v.Dependencies {
				if !seen[d.ToArtifactID] {
					seen[d.ToArtifactID] = true
					adj[a.ID] = append(adj[a.ID], d.ToArtifactID)
				}
			}
		}
	}
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[int64]int{}
	path := []int64{}
	var dfs func(id int64) []string
	dfs = func(id int64) []string {
		color[id] = gray
		path = append(path, id)
		for _, n := range adj[id] {
			if color[n] == gray {
				names := make([]string, 0, len(path)+1)
				start := 0
				for i, p := range path {
					if p == n {
						start = i
						break
					}
				}
				for _, p := range path[start:] {
					if a := ix.ArtifactByID(p); a != nil {
						names = append(names, a.Name)
					}
				}
				if a := ix.ArtifactByID(n); a != nil {
					names = append(names, a.Name)
				}
				path = path[:len(path)-1]
				return names
			}
			if color[n] == white {
				if r := dfs(n); r != nil {
					path = path[:len(path)-1]
					return r
				}
			}
		}
		color[id] = black
		path = path[:len(path)-1]
		return nil
	}
	return dfs(rootID)
}
