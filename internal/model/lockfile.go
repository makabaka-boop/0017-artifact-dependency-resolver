package model

import "time"

// Lockfile 表示一次解析结果被固化后的可复现版本锁定快照。
// 它与解析快照解耦：解析快照记录当时的输入与结果，而锁定快照只关心
// 「每个制品应安装哪个精确版本」，供后续以锁定约束重新解析。
type Lockfile struct {
	ID                 int64     `json:"id"`
	Name               string    `json:"name"`
	RootArtifactID     int64     `json:"root_artifact_id"`
	RootVersionID      *int64    `json:"root_version_id"`
	SourceResolutionID int64     `json:"source_resolution_id"`
	CreatedAt          time.Time `json:"created_at"`
}

// LockfileEntry 是锁定快照中的单条 pin 记录。
type LockfileEntry struct {
	ID              int64  `json:"id"`
	LockfileID      int64  `json:"lockfile_id"`
	ArtifactID      int64  `json:"artifact_id"`
	VersionID       int64  `json:"version_id"`
	SelectedVersion string `json:"selected_version"`
}

// LockfileStatus 枚举锁定的复核状态（用于「以锁定约束再次解析」的产出）。
const (
	LockfileInSync  = "in_sync"
	LockfileDrifted = "drifted"
)

// Statuses 扩展 resolution 来源枚举：重跑产生的快照也用独立来源标记，
// 以便与手动解析、回滚三方历史区分。
const (
	SourceRerun = "rerun"
)
