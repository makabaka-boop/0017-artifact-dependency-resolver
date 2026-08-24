package resolver

// Candidate 描述解析过程中针对某个目标制品尝试过的某个候选版本，
// 以及该候选被接受或被淘汰的原因。用于把「冲突/缺失」的诊断从一句话
// 展开为逐条可审阅的候选与淘汰依据。
type Candidate struct {
	Artifact string `json:"artifact"`
	Version  string `json:"version"`
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason"`
}

// trace 聚合一次解析中所有被尝试的候选版本与它们的淘汰/接受原因。
// 它由 resolveCtx 持有，在回溯失败路径上被填充到诊断明细中。
type trace struct {
	candidates []Candidate
}

func (t *trace) record(artifact, version string, accepted bool, reason string) {
	t.candidates = append(t.candidates, Candidate{
		Artifact: artifact,
		Version:  version,
		Accepted: accepted,
		Reason:   reason,
	})
}

// snapshot 返回候选明细的一份拷贝，供失败诊断置入 Candidates 字段。
func (t *trace) snapshot() []Candidate {
	if t == nil || len(t.candidates) == 0 {
		return nil
	}
	out := make([]Candidate, len(t.candidates))
	copy(out, t.candidates)
	return out
}
