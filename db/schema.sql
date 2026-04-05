CREATE TABLE IF NOT EXISTS learners (
    id            TEXT PRIMARY KEY,
    email         TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    objective     TEXT NOT NULL,
    webhook_url   TEXT DEFAULT '',
    profile_json  TEXT DEFAULT '{}',
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_active   DATETIME
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    token         TEXT PRIMARY KEY,
    learner_id    TEXT NOT NULL REFERENCES learners(id),
    expires_at    DATETIME NOT NULL,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS domains (
    id            TEXT PRIMARY KEY,
    learner_id    TEXT NOT NULL REFERENCES learners(id),
    name          TEXT NOT NULL,
    graph_json    TEXT NOT NULL,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS concept_states (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    learner_id     TEXT NOT NULL REFERENCES learners(id),
    concept        TEXT NOT NULL,
    stability      REAL DEFAULT 1.0,
    difficulty     REAL DEFAULT 0.3,
    elapsed_days   INTEGER DEFAULT 0,
    scheduled_days INTEGER DEFAULT 1,
    reps           INTEGER DEFAULT 0,
    lapses         INTEGER DEFAULT 0,
    card_state     TEXT DEFAULT 'new',
    last_review    DATETIME,
    next_review    DATETIME,
    p_mastery      REAL DEFAULT 0.1,
    p_learn        REAL DEFAULT 0.3,
    p_forget       REAL DEFAULT 0.05,
    p_slip         REAL DEFAULT 0.1,
    p_guess        REAL DEFAULT 0.2,
    theta          REAL DEFAULT 0.0,
    pfa_successes  REAL DEFAULT 0.0,
    pfa_failures   REAL DEFAULT 0.0,
    updated_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(learner_id, concept)
);

CREATE TABLE IF NOT EXISTS interactions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    learner_id    TEXT NOT NULL REFERENCES learners(id),
    concept       TEXT NOT NULL,
    activity_type TEXT NOT NULL,
    success       INTEGER NOT NULL,
    response_time INTEGER,
    confidence    REAL,
    error_type    TEXT DEFAULT '',
    notes         TEXT,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS availability (
    learner_id     TEXT PRIMARY KEY REFERENCES learners(id),
    windows_json   TEXT DEFAULT '[]',
    avg_duration   INTEGER DEFAULT 30,
    sessions_week  INTEGER DEFAULT 3,
    do_not_disturb INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS scheduled_alerts (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    learner_id    TEXT NOT NULL REFERENCES learners(id),
    alert_type    TEXT NOT NULL,
    concept       TEXT DEFAULT '',
    scheduled_at  DATETIME NOT NULL,
    sent          INTEGER DEFAULT 0,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
