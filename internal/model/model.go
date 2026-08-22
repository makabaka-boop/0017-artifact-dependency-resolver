package model

import (
	"regexp"
	"time"
)

// Artifact 表示一个软件制品。
type Artifact struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ArtifactVersion 表示某制品的一个已登记版本。
type ArtifactVersion struct {
	ID         int64     `json:"id"`
	ArtifactID int64     `json:"artifact_id"`
	Version    string    `json:"version"`
	CreatedAt  time.Time `json:"created_at"`
}

// Dependency 表示某个版本对其目标制品的约束性依赖。
type Dependency struct {
	ID            int64  `json:"id"`
	FromVersionID int64  `json:"from_version_id"`
	ToArtifactID  int64  `json:"to_artifact_id"`
	Constraint    string `json:"constraint"`
	CreatedAt     time.Time
}

// Resolution 表示一次解析请求及其结果快照。
type Resolution struct {
	ID                 int64     `json:"id"`
	RequestRef         string    `json:"request_ref"`
	RootArtifactID     int64     `json:"root_artifact_id"`
	RootVersionID      *int64    `json:"root_version_id"`
	InputJSON          string    `json:"input_json"`
	Status             string    `json:"status"`
	ResultJSON         string    `json:"result_json"`
	ErrorCode          string    `json:"error_code"`
	ErrorMessage       string    `json:"error_message"`
	Source             string    `json:"source"`
	SourceResolutionID *int64    `json:"source_resolution_id"`
	CreatedAt          time.Time `json:"created_at"`
}

// ResolutionItem 反规范化表示解析结果中的某个制品及其选定版本。
type ResolutionItem struct {
	ID              int64     `json:"id"`
	ResolutionID    int64     `json:"resolution_id"`
	ArtifactID      int64     `json:"artifact_id"`
	VersionID       int64     `json:"version_id"`
	SelectedVersion string    `json:"selected_version"`
	Depth           int       `json:"depth"`
	Reason          string    `json:"reason"`
	CreatedAt       time.Time `json:"created_at"`
}

// ChangeRecord 追加式变更留存记录。
type ChangeRecord struct {
	ID         int64     `json:"id"`
	EntityType string    `json:"entity_type"`
	EntityID   int64     `json:"entity_id"`
	Action     string    `json:"action"`
	BeforeJSON string    `json:"before_json"`
	AfterJSON  string    `json:"after_json"`
	CreatedAt  time.Time `json:"created_at"`
}

// 状态枚举。
const (
	ResolutionSucceeded = "succeeded"
	ResolutionFailed    = "failed"
	SourceDirect        = "direct"
	SourceRollback      = "rollback"
)

// ErrorCode 枚举（属于模型层以便各层共享）。
const (
	ErrBadRequest    = "bad_request"
	ErrNotFound      = "not_found"
	ErrConflict      = "conflict"
	ErrUnprocessable = "unprocessable"
	ErrInternal      = "internal"
	ErrNotReady      = "not_ready"
)

var artifactNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidArtifactName 校验制品名：小写字母、数字、短横线，不以短横线开头或结尾，长度≤128。
func ValidArtifactName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	return artifactNameRe.MatchString(name)
}
