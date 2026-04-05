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
		`ALTER TABLE interactions ADD COLUMN hints_requested INTEGER DEFAULT 0`,
		`ALTER TABLE interactions ADD COLUMN self_initiated INTEGER DEFAULT 0`,
		`ALTER TABLE interactions ADD COLUMN calibration_id TEXT DEFAULT ''`,
		`ALTER TABLE interactions ADD COLUMN is_proactive_review INTEGER DEFAULT 0`,
		`ALTER TABLE domains ADD COLUMN personal_goal TEXT DEFAULT ''`,
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
		`CREATE TABLE IF NOT EXISTS affect_states (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    learner_id           TEXT NOT NULL REFERENCES learners(id),
    session_id           TEXT NOT NULL,
    energy               INTEGER DEFAULT 0,
    subject_confidence   INTEGER DEFAULT 0,
    satisfaction         INTEGER DEFAULT 0,
    perceived_difficulty INTEGER DEFAULT 0,
    next_session_intent  INTEGER DEFAULT 0,
    autonomy_score       REAL DEFAULT 0,
    created_at           DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(learner_id, session_id)
)`,
		`CREATE TABLE IF NOT EXISTS calibration_records (
    prediction_id TEXT PRIMARY KEY,
    learner_id    TEXT NOT NULL REFERENCES learners(id),
    concept_id    TEXT NOT NULL,
    predicted     REAL NOT NULL,
    actual        REAL,
    delta         REAL,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
)`,
		`CREATE TABLE IF NOT EXISTS transfer_records (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    learner_id   TEXT NOT NULL REFERENCES learners(id),
    concept_id   TEXT NOT NULL,
    context_type TEXT NOT NULL,
    score        REAL NOT NULL,
    session_id   TEXT DEFAULT '',
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
)`,
		`CREATE INDEX IF NOT EXISTS idx_affect_states_learner ON affect_states(learner_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_calibration_records_learner ON calibration_records(learner_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_transfer_records_learner_concept ON transfer_records(learner_id, concept_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_interactions_self_initiated ON interactions(learner_id, self_initiated, created_at)`,
	}
	for _, m := range idempotentMigrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("idempotent migration: %w", err)
		}
	}
	return nil
}
