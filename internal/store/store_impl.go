package store

// NextID 返回用于 request_ref 等的单调递增 ID。
func (s *sqliteStore) NextID() (int64, error) {
	var id int64
	err := s.db.QueryRow(`SELECT COALESCE(MAX(id),0)+1 FROM resolutions`).Scan(&id)
	return id, err
}
