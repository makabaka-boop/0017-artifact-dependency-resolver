package store

import (
	"time"

	"artifact-dep-resolver/internal/model"
)

func (s *sqliteStore) AddChange(c *model.ChangeRecord, now time.Time) error {
	res, err := s.db.Exec(`INSERT INTO change_records(entity_type, entity_id, action, before_json, after_json, created_at) VALUES(?,?,?,?,?,?)`,
		c.EntityType, c.EntityID, c.Action, c.BeforeJSON, c.AfterJSON, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	c.ID, _ = res.LastInsertId()
	return nil
}

func (s *sqliteStore) ListChanges(limit, offset int, entityType string, entityID int64) ([]model.ChangeRecord, error) {
	q := `SELECT id, entity_type, entity_id, action, before_json, after_json, created_at FROM change_records`
	args := []interface{}{}
	conds := []string{}
	if entityType != "" {
		conds = append(conds, `entity_type = ?`)
		args = append(args, entityType)
	}
	if entityID > 0 {
		conds = append(conds, `entity_id = ?`)
		args = append(args, entityID)
	}
	if len(conds) > 0 {
		q += ` WHERE ` + joinCond(conds)
	}
	q += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ChangeRecord
	for rows.Next() {
		var c model.ChangeRecord
		var ca string
		if err := rows.Scan(&c.ID, &c.EntityType, &c.EntityID, &c.Action, &c.BeforeJSON, &c.AfterJSON, &ca); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *sqliteStore) GetChange(id int64) (*model.ChangeRecord, error) {
	var c model.ChangeRecord
	var ca string
	if err := s.db.QueryRow(`SELECT id, entity_type, entity_id, action, before_json, after_json, created_at FROM change_records WHERE id = ?`, id).
		Scan(&c.ID, &c.EntityType, &c.EntityID, &c.Action, &c.BeforeJSON, &c.AfterJSON, &ca); err != nil {
		return nil, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, ca)
	return &c, nil
}

func joinCond(conds []string) string {
	out := ""
	for i, c := range conds {
		if i > 0 {
			out += " AND "
		}
		out += c
	}
	return out
}
