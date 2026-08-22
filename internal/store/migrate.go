package store

import "database/sql"

// migration 维护 schema_version 并建表。
func migrate(db *sql.DB) error {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER NOT NULL
	);
	`)
	if err != nil {
		return err
	}
	var v int
	if err := db.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&v); err != nil {
		if err == sql.ErrNoRows {
			if _, err := db.Exec(`INSERT INTO schema_version(version) VALUES(0)`); err != nil {
				return err
			}
			v = 0
		} else {
			return err
		}
	}
	if v >= 1 {
		return nil
	}
	for _, stmt := range schemaStatements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`UPDATE schema_version SET version = 1`); err != nil {
		return err
	}
	return nil
}

var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS artifacts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS artifact_versions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		artifact_id INTEGER NOT NULL REFERENCES artifacts(id),
		version TEXT NOT NULL,
		created_at TEXT NOT NULL,
		UNIQUE(artifact_id, version)
	)`,
	`CREATE TABLE IF NOT EXISTS dependencies (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		from_version_id INTEGER NOT NULL REFERENCES artifact_versions(id),
		to_artifact_id INTEGER NOT NULL REFERENCES artifacts(id),
		"constraint" TEXT NOT NULL,
		created_at TEXT NOT NULL,
		UNIQUE(from_version_id, to_artifact_id)
	)`,
	`CREATE TABLE IF NOT EXISTS resolutions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		request_ref TEXT NOT NULL UNIQUE,
		root_artifact_id INTEGER NOT NULL REFERENCES artifacts(id),
		root_version_id INTEGER REFERENCES artifact_versions(id),
		input_json TEXT NOT NULL,
		status TEXT NOT NULL,
		result_json TEXT NOT NULL,
		error_code TEXT NOT NULL,
		error_message TEXT NOT NULL,
		source TEXT NOT NULL,
		source_resolution_id INTEGER REFERENCES resolutions(id),
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS resolution_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		resolution_id INTEGER NOT NULL REFERENCES resolutions(id),
		artifact_id INTEGER NOT NULL REFERENCES artifacts(id),
		version_id INTEGER NOT NULL REFERENCES artifact_versions(id),
		selected_version TEXT NOT NULL,
		depth INTEGER NOT NULL,
		reason TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS change_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		entity_type TEXT NOT NULL,
		entity_id INTEGER NOT NULL,
		action TEXT NOT NULL,
		before_json TEXT NOT NULL,
		after_json TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS lockfiles (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		root_artifact_id INTEGER NOT NULL REFERENCES artifacts(id),
		root_version_id INTEGER REFERENCES artifact_versions(id),
		source_resolution_id INTEGER NOT NULL REFERENCES resolutions(id),
		created_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS lockfile_entries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		lockfile_id INTEGER NOT NULL REFERENCES lockfiles(id),
		artifact_id INTEGER NOT NULL REFERENCES artifacts(id),
		version_id INTEGER NOT NULL REFERENCES artifact_versions(id),
		selected_version TEXT NOT NULL,
		UNIQUE(lockfile_id, artifact_id)
	)`,
}
