package db

import (
	"database/sql"
	"fmt"
	"testing"
)

// TestMigrate_Idempotent runs Migrate twice on a fresh in-memory database;
// the second invocation must be a no-op (no error, no duplicate-table or
// duplicate-column errors propagated). Then we assert that all expected
// tables and indexes exist by querying sqlite_master.
func TestMigrate_Idempotent(t *testing.T) {
	dsn := fmt.Sprintf("file:migrate_idempo_%d?mode=memory&cache=shared", testDBCounter+10000)
	testDBCounter++
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := Migrate(db); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate (must be idempotent): %v", err)
	}

	expectedTables := []string{
		"learners",
		"refresh_tokens",
		"domains",
		"concept_states",
		"interactions",
		"availability",
		"scheduled_alerts",
		"oauth_codes",
		"oauth_clients",
		"affect_states",
		"calibration_records",
		"transfer_records",
		"implementation_intentions",
		"webhook_message_queue",
	}
	for _, table := range expectedTables {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("expected table %q to exist: %v", table, err)
		}
	}

	expectedIndexes := []string{
		"idx_concept_states_learner",
		"idx_concept_states_review",
		"idx_interactions_learner_created",
		"idx_interactions_learner_concept",
		"idx_scheduled_alerts_learner_type",
		"idx_oauth_codes_expires",
		"idx_affect_states_learner",
		"idx_calibration_records_learner",
		"idx_transfer_records_learner_concept",
		"idx_interactions_self_initiated",
		"idx_interactions_misconception",
		"idx_impl_intent_learner",
		"idx_wmq_dispatch",
	}
	for _, idx := range expectedIndexes {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='index' AND name = ?`, idx,
		).Scan(&name)
		if err != nil {
			t.Errorf("expected index %q to exist: %v", idx, err)
		}
	}

	// Sanity: all migrated columns are queryable.
	if _, err := db.Exec(
		`INSERT INTO learners (id, email, password_hash, objective, profile_json) VALUES ('m1','m@m','h','o','{}')`,
	); err != nil {
		t.Fatalf("insert with profile_json: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO interactions (learner_id, concept, activity_type, success, error_type, hints_requested, self_initiated, calibration_id, is_proactive_review, misconception_type, misconception_detail) VALUES ('m1','C','RECALL_EXERCISE',1,'',0,0,'',0,NULL,NULL)`,
	); err != nil {
		t.Fatalf("insert with v0.9/v0.10 columns: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO domains (id, learner_id, name, graph_json, personal_goal, archived, value_framings_json, last_value_axis) VALUES ('d1','m1','dn','{}','goal',0,'','')`,
	); err != nil {
		t.Fatalf("insert with domain framing columns: %v", err)
	}
}

// TestOpenDB_Memory exercises the OpenDB helper. OpenDB appends `?_pragma=...`
// to the path before opening, so we use a file-backed temp DB to keep the DSN
// shape simple.
func TestOpenDB_Memory(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(dir + "/open.db")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

// TestOpenDB_BadPath ensures the error path is exercised when ping fails.
func TestOpenDB_BadPath(t *testing.T) {
	// "/proc/0/forbidden" is not openable as a file-backed sqlite db on Linux.
	_, err := OpenDB("/proc/0/forbidden-not-a-real-sqlite-file")
	if err == nil {
		t.Fatal("expected error for unreachable path")
	}
}
