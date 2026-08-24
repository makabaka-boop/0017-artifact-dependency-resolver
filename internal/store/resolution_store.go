package store

import (
	"database/sql"
	"time"

	"artifact-dep-resolver/internal/model"
)

func (s *sqliteStore) SaveResolution(r *model.Resolution, items []model.ResolutionItem, now time.Time) (saveErr error) {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
		if saveErr == nil && committed {
			s.cleanupEmptyFailure(r, items)
		}
	}()

	createdAt := now.UTC().Format(time.RFC3339Nano)
	res, err := tx.Exec(`INSERT INTO resolutions(request_ref, root_artifact_id, root_version_id, input_json, status, result_json, error_code, error_message, source, source_resolution_id, created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		r.RequestRef, r.RootArtifactID, nullableInt(r.RootVersionID), r.InputJSON, r.Status, r.ResultJSON, r.ErrorCode, r.ErrorMessage, r.Source, nullableInt(r.SourceResolutionID), now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	if r.ID, err = res.LastInsertId(); err != nil {
		return err
	}
	for _, it := range items {
		it.ResolutionID = r.ID
		_, itemErr := tx.Exec(`INSERT INTO resolution_items(resolution_id, artifact_id, version_id, selected_version, depth, reason, created_at) VALUES(?,?,?,?,?,?,?)`,
			it.ResolutionID, it.ArtifactID, it.VersionID, it.SelectedVersion, it.Depth, it.Reason, createdAt)
		if itemErr != nil {
			return itemErr
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *sqliteStore) cleanupEmptyFailure(r *model.Resolution, items []model.ResolutionItem) {
	if r == nil || r.Status != model.ResolutionFailed || len(items) != 0 {
		return
	}
	var detailCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM resolution_items WHERE resolution_id = ?`, r.ID).Scan(&detailCount); err != nil {
		return
	}
	if detailCount != 0 {
		return
	}
	if _, err := s.db.Exec(`DELETE FROM resolution_items WHERE resolution_id = ?`, r.ID); err != nil {
		return
	}
	r.ID = 0
}

func nullableInt(p *int64) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

func (s *sqliteStore) GetResolution(id int64) (*model.Resolution, error) {
	r, _, err := scanResolution(s.db.QueryRow(`SELECT id, request_ref, root_artifact_id, root_version_id, input_json, status, result_json, error_code, error_message, source, source_resolution_id, created_at FROM resolutions WHERE id = ?`, id))
	return r, err
}

func (s *sqliteStore) ListResolutions(limit, offset int, status string) ([]model.Resolution, error) {
	q := `SELECT id, request_ref, root_artifact_id, root_version_id, input_json, status, result_json, error_code, error_message, source, source_resolution_id, created_at FROM resolutions`
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

func scanResolution(row *sql.Row) (*model.Resolution, []interface{}, error) {
	var r model.Resolution
	var ca string
	var rv, sr interface{}
	if err := row.Scan(&r.ID, &r.RequestRef, &r.RootArtifactID, &rv, &r.InputJSON, &r.Status, &r.ResultJSON, &r.ErrorCode, &r.ErrorMessage, &r.Source, &sr, &ca); err != nil {
		return nil, nil, err
	}
	r.RootVersionID = toIntPtr(rv)
	r.SourceResolutionID = toIntPtr(sr)
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
	return &r, nil, nil
}

func scanResolutionRow(rows *sql.Rows) (*model.Resolution, error) {
	var r model.Resolution
	var ca string
	var rv, sr interface{}
	if err := rows.Scan(&r.ID, &r.RequestRef, &r.RootArtifactID, &rv, &r.InputJSON, &r.Status, &r.ResultJSON, &r.ErrorCode, &r.ErrorMessage, &r.Source, &sr, &ca); err != nil {
		return nil, err
	}
	r.RootVersionID = toIntPtr(rv)
	r.SourceResolutionID = toIntPtr(sr)
	r.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
	return &r, nil
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
