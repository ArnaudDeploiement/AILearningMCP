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

CREATE TABLE IF NOT EXISTS oauth_codes (
    code           TEXT PRIMARY KEY,
    learner_id     TEXT NOT NULL REFERENCES learners(id),
    code_challenge TEXT NOT NULL,
    expires_at     DATETIME NOT NULL,
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS oauth_clients (
    client_id      TEXT PRIMARY KEY,
    client_name    TEXT DEFAULT '',
    redirect_uris  TEXT DEFAULT '[]',
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Metacognitive loop tables (v0.9)

CREATE TABLE IF NOT EXISTS affect_states (
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
);

CREATE TABLE IF NOT EXISTS calibration_records (
    prediction_id TEXT PRIMARY KEY,
    learner_id    TEXT NOT NULL REFERENCES learners(id),
    concept_id    TEXT NOT NULL,
    predicted     REAL NOT NULL,
    actual        REAL,
    delta         REAL,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS transfer_records (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    learner_id   TEXT NOT NULL REFERENCES learners(id),
    concept_id   TEXT NOT NULL,
    context_type TEXT NOT NULL,
    score        REAL NOT NULL,
    session_id   TEXT DEFAULT '',
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_concept_states_learner
    ON concept_states(learner_id);

CREATE INDEX IF NOT EXISTS idx_concept_states_review
    ON concept_states(learner_id, next_review);

CREATE INDEX IF NOT EXISTS idx_interactions_learner_created
    ON interactions(learner_id, created_at);

CREATE INDEX IF NOT EXISTS idx_interactions_learner_concept
    ON interactions(learner_id, concept, created_at);

CREATE INDEX IF NOT EXISTS idx_scheduled_alerts_learner_type
    ON scheduled_alerts(learner_id, alert_type, created_at);

CREATE INDEX IF NOT EXISTS idx_oauth_codes_expires
    ON oauth_codes(expires_at);

CREATE INDEX IF NOT EXISTS idx_affect_states_learner
    ON affect_states(learner_id, created_at);

CREATE INDEX IF NOT EXISTS idx_calibration_records_learner
    ON calibration_records(learner_id, created_at);

CREATE INDEX IF NOT EXISTS idx_transfer_records_learner_concept
    ON transfer_records(learner_id, concept_id, created_at);

CREATE INDEX IF NOT EXISTS idx_interactions_self_initiated
    ON interactions(learner_id, self_initiated, created_at);
