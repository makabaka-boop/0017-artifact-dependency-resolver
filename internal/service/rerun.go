package service

import (
	"encoding/json"

	"artifact-dep-resolver/internal/errcode"
	"artifact-dep-resolver/internal/model"
)

// resolveInput 是解析原始请求的结构，用于重跑时复用原清单。
type resolveInput struct {
	Artifact string  `json:"artifact"`
	Version  *string `json:"version"`
}

// RerunResolution 按 resolution_id 复用原请求的根制品与版本，重新执行解析。
// 重跑不修改原快照，而是生成 source=rerun 的新快照并记录变更留存。
func (s *Service) RerunResolution(id int64) (*ResolutionOutput, error) {
	orig, err := s.store.GetResolution(id)
	if err != nil {
		return nil, errcode.New(errcode.NotFound, "resolution not found")
	}
	var input resolveInput
	if err := json.Unmarshal([]byte(orig.InputJSON), &input); err != nil {
		return nil, errcode.New(errcode.Internal, "invalid original input json")
	}
	if input.Artifact == "" {
		return nil, errcode.New(errcode.Unprocessable, "original request has no root artifact")
	}

	srcID := orig.ID
	out, err := s.resolveForSource(input.Artifact, input.Version, model.SourceRerun, &srcID)
	if err != nil {
		return nil, err
	}
	if err := s.recordChange("resolution", out.ResolutionID, "rerun", mustJSON(orig), mustJSON(out)); err != nil {
		return nil, err
	}
	return out, nil
}
