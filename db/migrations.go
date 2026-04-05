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
	return nil
}
