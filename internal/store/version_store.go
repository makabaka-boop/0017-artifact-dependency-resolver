package store

import (
	"database/sql"
	"time"

	"artifact-dep-resolver/internal/model"
)

func (s *sqliteStore) CreateVersion(artifactID int64, version string, now time.Time) (*model.ArtifactVersion, error) {
	res, err := s.db.Exec(`INSERT INTO artifact_versions(artifact_id, version, created_at) VALUES(?,?,?)`, artifactID, version, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetVersionByID(id)
}

func (s *sqliteStore) GetVersion(artifactID int64, version string) (*model.ArtifactVersion, error) {
	return scanVersion(s.db.QueryRow(`SELECT id, artifact_id, version, created_at FROM artifact_versions WHERE artifact_id = ? AND version = ?`, artifactID, version))
}

func (s *sqliteStore) GetVersionByID(id int64) (*model.ArtifactVersion, error) {
	return scanVersion(s.db.QueryRow(`SELECT id, artifact_id, version, created_at FROM artifact_versions WHERE id = ?`, id))
}

func (s *sqliteStore) ListVersions(artifactID int64) ([]model.ArtifactVersion, error) {
	rows, err := s.db.Query(`SELECT id, artifact_id, version, created_at FROM artifact_versions WHERE artifact_id = ? ORDER BY id`, artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ArtifactVersion
	for rows.Next() {
		var v model.ArtifactVersion
		var ca string
		if err := rows.Scan(&v.ID, &v.ArtifactID, &v.Version, &ca); err != nil {
			return nil, err
		}
		v.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
		out = append(out, v)
	}
	return out, rows.Err()
}

func scanVersion(row *sql.Row) (*model.ArtifactVersion, error) {
	var v model.ArtifactVersion
	var ca string
	if err := row.Scan(&v.ID, &v.ArtifactID, &v.Version, &ca); err != nil {
		return nil, err
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
	return &v, nil
}

func (s *sqliteStore) ReplaceDependencies(versionID int64, deps []model.Dependency, now time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM dependencies WHERE from_version_id = ?`, versionID); err != nil {
		return err
	}
	for _, d := range deps {
		if _, err := tx.Exec(`INSERT INTO dependencies(from_version_id, to_artifact_id, "constraint", created_at) VALUES(?,?,?,?)`,
			versionID, d.ToArtifactID, d.Constraint, now.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *sqliteStore) ListDependencies(versionID int64) ([]model.Dependency, error) {
	rows, err := s.db.Query(`SELECT id, from_version_id, to_artifact_id, "constraint", created_at FROM dependencies WHERE from_version_id = ? ORDER BY to_artifact_id`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Dependency
	for rows.Next() {
		var d model.Dependency
		var ca string
		if err := rows.Scan(&d.ID, &d.FromVersionID, &d.ToArtifactID, &d.Constraint, &ca); err != nil {
			return nil, err
		}
		d.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
		out = append(out, d)
	}
	return out, rows.Err()
}
