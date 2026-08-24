package resolver

import (
	"sort"

	"artifact-dep-resolver/internal/constraint"
	"artifact-dep-resolver/internal/semver"
)

// Artifact contains the resolution-relevant view of an artifact and its versions.
type Artifact struct {
	ID       int64
	Name     string
	Versions []ArtifactVersion
}

// ArtifactVersion is a version plus its dependencies.
type ArtifactVersion struct {
	ID           int64
	Version      string
	Ver          semver.Version
	Dependencies []Dep
}

// Dep maps to a target artifact with a constraint.
type Dep struct {
	ToArtifactID int64
	ToName       string
	Constraint   constraint.Constraint
}

// Selected records a chosen version for an artifact during resolution.
type Selected struct {
	ArtifactID   int64
	ArtifactName string
	VersionID    int64
	Version      string
	Depth        int
	Reason       string
}

// Diagnostic describes a failure reason.
type Diagnostic struct {
	Type       string      `json:"type"`
	Message    string      `json:"message"`
	Details    []string    `json:"details"`
	Candidates []Candidate `json:"candidates,omitempty"`
}

// Result is the outcome of a resolution attempt.
type Result struct {
	Status      string
	Resolved    []Selected
	Diagnostics []Diagnostic
}

// Index maps artifact ID/name to artifact.
type Index struct {
	byID   map[int64]*Artifact
	byName map[string]*Artifact
}

// NewIndex builds an index with versions sorted highest-first.
func NewIndex(artifacts []Artifact) *Index {
	ix := &Index{byID: map[int64]*Artifact{}, byName: map[string]*Artifact{}}
	for i := range artifacts {
		a := &artifacts[i]
		sort.SliceStable(a.Versions, func(x, y int) bool {
			return a.Versions[x].Ver.Compare(a.Versions[y].Ver) > 0
		})
		ix.byID[a.ID] = a
		ix.byName[a.Name] = a
	}
	return ix
}

// ArtifactByID returns the artifact by id.
func (ix *Index) ArtifactByID(id int64) *Artifact { return ix.byID[id] }

// ArtifactByName returns the artifact by name.
func (ix *Index) ArtifactByName(name string) *Artifact { return ix.byName[name] }

// Resolve runs the backtracking algorithm.
func Resolve(ix *Index, rootName string, rootVersion *string) Result {
	root := ix.ArtifactByName(rootName)
	if root == nil {
		return Result{Status: "failed", Diagnostics: []Diagnostic{{Type: "MISSING", Message: "root artifact not found", Details: []string{rootName}}}}
	}
	if len(root.Versions) == 0 {
		return Result{Status: "failed", Diagnostics: []Diagnostic{{Type: "MISSING", Message: "no versions for root artifact", Details: []string{rootName}}}}
	}

	// 环检测（基于所有版本的依赖并集）。
	if cycle := detectCycle(ix, root.ID); cycle != nil {
		return Result{Status: "failed", Diagnostics: []Diagnostic{{Type: "CYCLE", Message: "dependency cycle detected", Details: cycle}}}
	}

	ctx := &resolveCtx{
		ix:       ix,
		assigned: map[int64]int{},
		depth:    map[int64]int{},
	}

	// 根版本选择。
	candidates := rootCandidateIndexes(root, rootVersion)
	for _, ci := range candidates {
		ctx.assign(root.ID, ci, 0)
		if ctx.search() {
			break
		}
		ctx.unassign(root.ID)
	}
	if _, ok := ctx.assigned[root.ID]; !ok {
		if ctx.fail.Type == "" {
			ctx.fail = Diagnostic{Type: "CONFLICT", Message: "no version of root satisfies all constraints", Details: []string{root.Name}}
		}
		if len(ctx.fail.Candidates) == 0 {
			ctx.fail.Candidates = ctx.trace.snapshot()
		}
		return Result{Status: "failed", Diagnostics: []Diagnostic{ctx.fail}}
	}

	resolved := make([]Selected, 0, len(ctx.order))
	ids := ctx.orderedIDs()
	for _, aid := range ids {
		a := ix.ArtifactByID(aid)
		if a == nil {
			continue
		}
		vi := ctx.assigned[aid]
		v := a.Versions[vi]
		resolved = append(resolved, Selected{
			ArtifactID:   a.ID,
			ArtifactName: a.Name,
			VersionID:    v.ID,
			Version:      v.Version,
			Depth:        ctx.depth[aid],
			Reason:       "selected",
		})
	}
	sort.SliceStable(resolved, func(i, j int) bool {
		if resolved[i].Depth != resolved[j].Depth {
			return resolved[i].Depth < resolved[j].Depth
		}
		return resolved[i].ArtifactName < resolved[j].ArtifactName
	})
	return Result{Status: "succeeded", Resolved: resolved}
}

// resolveCtx 跟踪一次解析的临时状态。
type resolveCtx struct {
	ix       *Index
	assigned map[int64]int
	depth    map[int64]int
	order    []int64
	fail     Diagnostic
	trace    trace
}

func (c *resolveCtx) assign(id int64, vi, depth int) {
	if _, ok := c.assigned[id]; !ok {
		c.order = append(c.order, id)
	}
	c.assigned[id] = vi
	c.depth[id] = depth
}

func (c *resolveCtx) unassign(id int64) {
	delete(c.assigned, id)
	delete(c.depth, id)
	for i, oid := range c.order {
		if oid == id {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
}

func (c *resolveCtx) orderedIDs() []int64 {
	return c.order
}

func rootCandidateIndexes(root *Artifact, rootVersion *string) []int {
	indexes := make([]int, 0, len(root.Versions))
	if rootVersion != nil {
		if v, err := semver.Parse(*rootVersion); err == nil {
			for i := range root.Versions {
				if root.Versions[i].Ver.Compare(v) == 0 {
					return append(indexes, i)
				}
			}
		}
	}
	for i := range root.Versions {
		indexes = append(indexes, i)
	}
	return indexes
}

// search 在工作清单上穷举回溯，冲突时回溯到最近的相关决定。
func (c *resolveCtx) search() bool {
	for _, aid := range c.orderedIDs() {
		a := c.ix.ArtifactByID(aid)
		if a == nil {
			continue
		}
		vi := c.assigned[aid]
		for _, d := range a.Versions[vi].Dependencies {
			target := c.ix.ArtifactByID(d.ToArtifactID)
			if target == nil {
				c.fail = Diagnostic{Type: "MISSING", Message: "dependency target missing", Details: []string{d.ToName}}
				return false
			}
			if prevIdx, ok := c.assigned[target.ID]; ok {
				if !d.Constraint.Matches(target.Versions[prevIdx].Ver) {
					c.trace.record(target.Name, target.Versions[prevIdx].Version, false,
						"already assigned version violates constraint "+d.Constraint.String())
					c.fail = Diagnostic{Type: "CONFLICT", Message: "selected version violates constraint", Details: []string{target.Name, target.Versions[prevIdx].Version}}
					return false
				}
				continue
			}
			// 目标未赋值：按顺序选出满足约束的版本，并记录每个候选的淘汰/接受原因。
			solved := false
			matchedCount := 0
			for tvi := range target.Versions {
				cand := target.Versions[tvi]
				if !d.Constraint.Matches(cand.Ver) {
					c.trace.record(target.Name, cand.Version, false,
						"version does not match constraint "+d.Constraint.String())
					continue
				}
				matchedCount++
				depth := c.depth[aid] + 1
				c.assign(target.ID, tvi, depth)
				if c.search() {
					c.trace.record(target.Name, cand.Version, true, "satisfies "+d.Constraint.String())
					solved = true
					break
				}
				c.unassign(target.ID)
				c.trace.record(target.Name, cand.Version, false,
					"version satisfies constraint but downstream resolution failed")
			}
			if !solved {
				if c.fail.Type == "" {
					msg := "no version satisfies constraint"
					if matchedCount > 0 {
						msg = "all candidate versions lead to downstream conflict"
					}
					c.fail = Diagnostic{Type: "MISSING", Message: msg, Details: []string{target.Name}}
				}
				c.fail.Candidates = c.trace.snapshot()
				return false
			}
		}
	}
	return true
}
