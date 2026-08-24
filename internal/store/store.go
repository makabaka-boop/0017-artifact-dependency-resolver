package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"artifact-dep-resolver/internal/model"
)

// Store 定义持久化仓储接口。
type Store interface {
	Close() error
	Ping() error

	CreateArtifact(name string, now time.Time) (*model.Artifact, error)
	GetArtifactByName(name string) (*model.Artifact, error)
	GetArtifactByID(id int64) (*model.Artifact, error)
	ListArtifacts(limit, offset int) ([]model.Artifact, error)

	CreateVersion(artifactID int64, version string, now time.Time) (*model.ArtifactVersion, error)
	GetVersion(artifactID int64, version string) (*model.ArtifactVersion, error)
	GetVersionByID(id int64) (*model.ArtifactVersion, error)
	ListVersions(artifactID int64) ([]model.ArtifactVersion, error)

	ReplaceDependencies(versionID int64, deps []model.Dependency, now time.Time) error
	ListDependencies(versionID int64) ([]model.Dependency, error)

	SaveResolution(r *model.Resolution, items []model.ResolutionItem, now time.Time) error
	GetResolution(id int64) (*model.Resolution, error)
	ListResolutions(limit, offset int, status string) ([]model.Resolution, error)
	ListResolutionItems(resolutionID int64) ([]model.ResolutionItem, error)

	AddChange(c *model.ChangeRecord, now time.Time) error
	ListChanges(limit, offset int, entityType string, entityID int64) ([]model.ChangeRecord, error)
	GetChange(id int64) (*model.ChangeRecord, error)

	SaveLockfile(l *model.Lockfile, entries []model.LockfileEntry, now time.Time) error
	GetLockfileByName(name string) (*model.Lockfile, error)
	GetLockfileByID(id int64) (*model.Lockfile, error)
	ListLockfiles(limit, offset int) ([]model.Lockfile, error)
	ListLockfileEntries(lockfileID int64) ([]model.LockfileEntry, error)

	LoadGraph() ([]GraphData, error)
	NextID() (int64, error)
}

// GraphData 提供解析所需的完整数据视图。
type GraphData struct {
	Artifact model.Artifact
	Versions []VersionData
}

// VersionData 携带版本及其依赖。
type VersionData struct {
	Version model.ArtifactVersion
	Deps    []DepData
}

// DepData 携带目标制品与约束。
type DepData struct {
	ToArtifactID int64
	ToName       string
	Constraint   string
}

// NextID 返回用于 request_ref 等的单调递增 ID（实现见 store_impl.go）。

type sqliteStore struct {
	db *sql.DB
}

var _ Store = (*sqliteStore)(nil)

// Open 打开（必要时初始化）SQLite 数据库。
func Open(path string) (*sqliteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := pragmas(db); err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		return nil, err
	}
	return &sqliteStore{db: db}, nil
}

func pragmas(db *sql.DB) error {
	stmts := []string{
		`PRAGMA foreign_keys = ON`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA busy_timeout = 5000`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("pragma %q: %w", s, err)
		}
	}
	return nil
}

func (s *sqliteStore) Close() error { return s.db.Close() }
func (s *sqliteStore) Ping() error  { return s.db.Ping() }
