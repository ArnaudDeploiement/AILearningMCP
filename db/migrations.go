package db

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

func OpenDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return db, nil
}

func Migrate(db *sql.DB) error {
	_, err := db.Exec(schemaSQL)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	// Incremental migrations for existing databases (ALTER TABLE is idempotent-safe)
	alterMigrations := []string{
		`ALTER TABLE learners ADD COLUMN profile_json TEXT DEFAULT '{}'`,
		`ALTER TABLE interactions ADD COLUMN error_type TEXT DEFAULT ''`,
	}
	for _, m := range alterMigrations {
		_, _ = db.Exec(m) // ignore "duplicate column" errors
	}

	// Idempotent table + index creation for existing databases
	idempotentMigrations := []string{
		`CREATE TABLE IF NOT EXISTS oauth_codes (
			code           TEXT PRIMARY KEY,
			learner_id     TEXT NOT NULL REFERENCES learners(id),
			code_challenge TEXT NOT NULL,
			expires_at     DATETIME NOT NULL,
			created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_clients (
			client_id      TEXT PRIMARY KEY,
			client_name    TEXT DEFAULT '',
			redirect_uris  TEXT DEFAULT '[]',
			created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_concept_states_learner ON concept_states(learner_id)`,
		`CREATE INDEX IF NOT EXISTS idx_concept_states_review ON concept_states(learner_id, next_review)`,
		`CREATE INDEX IF NOT EXISTS idx_interactions_learner_created ON interactions(learner_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_interactions_learner_concept ON interactions(learner_id, concept, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_scheduled_alerts_learner_type ON scheduled_alerts(learner_id, alert_type, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_codes_expires ON oauth_codes(expires_at)`,
	}
	for _, m := range idempotentMigrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("idempotent migration: %w", err)
		}
	}
	return nil
}
