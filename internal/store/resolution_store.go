package store

import (
	"database/sql"
	"time"

	"artifact-dep-resolver/internal/model"
)

const resolutionSelect = `SELECT id, request_ref, root_artifact_id, root_version_id, input_json, status, result_json, error_code, error_message, source, source_resolution_id, created_at FROM resolutions`

func (s *sqliteStore) SaveResolution(r *model.Resolution, items []model.ResolutionItem, now time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO resolutions(request_ref, root_artifact_id, root_version_id, input_json, status, result_json, error_code, error_message, source, source_resolution_id, created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		r.RequestRef, r.RootArtifactID, nullableInt(r.RootVersionID), r.InputJSON, r.Status, r.ResultJSON, r.ErrorCode, r.ErrorMessage, r.Source, nullableInt(r.SourceResolutionID), now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	r.ID, _ = res.LastInsertId()
	for _, it := range items {
		it.ResolutionID = r.ID
		if _, err := tx.Exec(`INSERT INTO resolution_items(resolution_id, artifact_id, version_id, selected_version, depth, reason, created_at) VALUES(?,?,?,?,?,?,?)`,
			it.ResolutionID, it.ArtifactID, it.VersionID, it.SelectedVersion, it.Depth, it.Reason, now.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func nullableInt(p *int64) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

func (s *sqliteStore) GetResolution(id int64) (*model.Resolution, error) {
	row := s.db.QueryRow(resolutionSelect+` WHERE id = ?`, id)
	r, _, err := scanResolution(row)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (s *sqliteStore) ListResolutions(limit, offset int, status string) ([]model.Resolution, error) {
	q := resolutionSelect
	args := []interface{}{}
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Resolution
	for rows.Next() {
		r, err := scanResolutionRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func (s *sqliteStore) ListResolutionItems(resolutionID int64) ([]model.ResolutionItem, error) {
	rows, err := s.db.Query(`SELECT id, resolution_id, artifact_id, version_id, selected_version, depth, reason, created_at FROM resolution_items WHERE resolution_id = ? ORDER BY depth, artifact_id`, resolutionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ResolutionItem
	for rows.Next() {
		var it model.ResolutionItem
		var ca string
		if err := rows.Scan(&it.ID, &it.ResolutionID, &it.ArtifactID, &it.VersionID, &it.SelectedVersion, &it.Depth, &it.Reason, &ca); err != nil {
			return nil, err
		}
		it.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
		out = append(out, it)
	}
	return out, rows.Err()
}

type resolutionRowScanner interface {
	Scan(dest ...interface{}) error
}

func scanResolutionFields(scanner resolutionRowScanner) (*model.Resolution, error) {
	var r model.Resolution
	var ca string
	var rv, sr interface{}
	if err := scanner.Scan(&r.ID, &r.RequestRef, &r.RootArtifactID, &rv, &r.InputJSON, &r.Status, &r.ResultJSON, &r.ErrorCode, &r.ErrorMessage, &r.Source, &sr, &ca); err != nil {
		return nil, err
	}
	r.RootVersionID = toIntPtr(rv)
	r.SourceResolutionID = toIntPtr(sr)
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
	return &r, nil
}

func scanResolution(row *sql.Row) (*model.Resolution, []interface{}, error) {
	r, err := scanResolutionFields(row)
	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return r, nil, nil
}

func scanResolutionRow(rows *sql.Rows) (*model.Resolution, error) {
	return scanResolutionFields(rows)
}

func toIntPtr(v interface{}) *int64 {
	switch n := v.(type) {
	case int64:
		return &n
	case nil:
		return nil
	}
	return nil
}
