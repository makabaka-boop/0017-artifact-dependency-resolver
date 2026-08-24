package store

import (
	"database/sql"
	"time"

	"artifact-dep-resolver/internal/model"
)

func (s *sqliteStore) CreateArtifact(name string, now time.Time) (*model.Artifact, error) {
	res, err := s.db.Exec(`INSERT INTO artifacts(name, created_at, updated_at) VALUES(?,?,?)`, name, now.UTC().Format(time.RFC3339Nano), now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetArtifactByID(id)
}

func (s *sqliteStore) GetArtifactByName(name string) (*model.Artifact, error) {
	return scanArtifact(s.db.QueryRow(`SELECT id, name, created_at, updated_at FROM artifacts WHERE name = ?`, name))
}

func (s *sqliteStore) GetArtifactByID(id int64) (*model.Artifact, error) {
	return scanArtifact(s.db.QueryRow(`SELECT id, name, created_at, updated_at FROM artifacts WHERE id = ?`, id))
}

func (s *sqliteStore) ListArtifacts(limit, offset int) ([]model.Artifact, error) {
	rows, err := s.db.Query(`SELECT id, name, created_at, updated_at FROM artifacts ORDER BY id LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Artifact
	for rows.Next() {
		a, err := scanArtifactRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func scanArtifact(row *sql.Row) (*model.Artifact, error) {
	var a model.Artifact
	var ca, ua string
	if err := row.Scan(&a.ID, &a.Name, &ca, &ua); err != nil {
		return nil, err
	}
	a.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
	a.UpdatedAt, _ = time.Parse(time.RFC3339Nano, ua)
	return &a, nil
}

func scanArtifactRow(rows *sql.Rows) (model.Artifact, error) {
	var a model.Artifact
	var ca, ua string
	if err := rows.Scan(&a.ID, &a.Name, &ca, &ua); err != nil {
		return a, err
	}
	a.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
	a.UpdatedAt, _ = time.Parse(time.RFC3339Nano, ua)
	return a, nil
}
