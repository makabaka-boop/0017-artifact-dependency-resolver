package store

import (
	"database/sql"
	"time"

	"artifact-dep-resolver/internal/model"
)

// SaveLockfile 持久化一个锁定快照及其条目（同一事务）。
func (s *sqliteStore) SaveLockfile(l *model.Lockfile, entries []model.LockfileEntry, now time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO lockfiles(name, root_artifact_id, root_version_id, source_resolution_id, created_at) VALUES(?,?,?,?,?)`,
		l.Name, l.RootArtifactID, nullableInt(l.RootVersionID), l.SourceResolutionID, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	l.ID, _ = res.LastInsertId()
	for _, e := range entries {
		e.LockfileID = l.ID
		if _, err := tx.Exec(`INSERT INTO lockfile_entries(lockfile_id, artifact_id, version_id, selected_version) VALUES(?,?,?,?)`,
			e.LockfileID, e.ArtifactID, e.VersionID, e.SelectedVersion); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetLockfileByName 按名查询锁定快照。
func (s *sqliteStore) GetLockfileByName(name string) (*model.Lockfile, error) {
	return scanLockfile(s.db.QueryRow(`SELECT id, name, root_artifact_id, root_version_id, source_resolution_id, created_at FROM lockfiles WHERE name = ?`, name))
}

// GetLockfileByID 按 id 查询锁定快照。
func (s *sqliteStore) GetLockfileByID(id int64) (*model.Lockfile, error) {
	return scanLockfile(s.db.QueryRow(`SELECT id, name, root_artifact_id, root_version_id, source_resolution_id, created_at FROM lockfiles WHERE id = ?`, id))
}

// ListLockfiles 分页列出锁定快照。
func (s *sqliteStore) ListLockfiles(limit, offset int) ([]model.Lockfile, error) {
	rows, err := s.db.Query(`SELECT id, name, root_artifact_id, root_version_id, source_resolution_id, created_at FROM lockfiles ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Lockfile
	for rows.Next() {
		l, err := scanLockfileRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

// ListLockfileEntries 列出某锁定快照的全部 pin 条目。
func (s *sqliteStore) ListLockfileEntries(lockfileID int64) ([]model.LockfileEntry, error) {
	rows, err := s.db.Query(`SELECT id, lockfile_id, artifact_id, version_id, selected_version FROM lockfile_entries WHERE lockfile_id = ? ORDER BY artifact_id`, lockfileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.LockfileEntry
	for rows.Next() {
		var e model.LockfileEntry
		if err := rows.Scan(&e.ID, &e.LockfileID, &e.ArtifactID, &e.VersionID, &e.SelectedVersion); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanLockfile(row *sql.Row) (*model.Lockfile, error) {
	var l model.Lockfile
	var ca string
	var rv interface{}
	if err := row.Scan(&l.ID, &l.Name, &l.RootArtifactID, &rv, &l.SourceResolutionID, &ca); err != nil {
		return nil, err
	}
	l.RootVersionID = toIntPtr(rv)
	l.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
	return &l, nil
}

func scanLockfileRow(rows *sql.Rows) (*model.Lockfile, error) {
	var l model.Lockfile
	var ca string
	var rv interface{}
	if err := rows.Scan(&l.ID, &l.Name, &l.RootArtifactID, &rv, &l.SourceResolutionID, &ca); err != nil {
		return nil, err
	}
	l.RootVersionID = toIntPtr(rv)
	l.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
	return &l, nil
}
