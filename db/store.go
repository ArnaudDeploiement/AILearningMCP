package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"learning-runtime/models"
)

// IsSafeWebhookURL validates that a webhook URL targets Discord over HTTPS.
// SSRF guard: only Discord webhook hosts allowed (blocks IMDS, internal ranges, etc.).
func IsSafeWebhookURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "https" {
		return false
	}
	host := u.Hostname()
	if host == "" || strings.Contains(host, "..") {
		return false
	}
	if net.ParseIP(host) != nil {
		return false
	}
	host = strings.ToLower(host)
	switch host {
	case "discord.com", "discordapp.com":
		return true
	}
	return strings.HasSuffix(host, ".discord.com") || strings.HasSuffix(host, ".discordapp.com")
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(b)
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ─── Learners ────────────────────────────────────────────────────────────────

func (s *Store) CreateLearner(email, passwordHash, objective, webhookURL string) (*models.Learner, error) {
	if webhookURL != "" && !IsSafeWebhookURL(webhookURL) {
		return nil, fmt.Errorf("invalid webhook_url: must be https://discord.com/...")
	}
	id := generateID()
	now := time.Now().UTC()
	_, err := s.db.Exec(
		`INSERT INTO learners (id, email, password_hash, objective, webhook_url, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, email, passwordHash, objective, webhookURL, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create learner: %w", err)
	}
	return &models.Learner{
		ID:           id,
		Email:        email,
		PasswordHash: passwordHash,
		Objective:    objective,
		WebhookURL:   webhookURL,
		CreatedAt:    now,
	}, nil
}

func (s *Store) GetLearnerByID(id string) (*models.Learner, error) {
	row := s.db.QueryRow(
		`SELECT id, email, password_hash, objective, webhook_url, profile_json, created_at, last_active
		 FROM learners WHERE id = ?`, id,
	)
	return scanLearner(row)
}

func (s *Store) GetLearnerByEmail(email string) (*models.Learner, error) {
	row := s.db.QueryRow(
		`SELECT id, email, password_hash, objective, webhook_url, profile_json, created_at, last_active
		 FROM learners WHERE email = ?`, email,
	)
	return scanLearner(row)
}

func scanLearner(row *sql.Row) (*models.Learner, error) {
	l := &models.Learner{}
	var lastActive sql.NullTime
	var profileJSON sql.NullString
	err := row.Scan(
		&l.ID, &l.Email, &l.PasswordHash, &l.Objective, &l.WebhookURL, &profileJSON, &l.CreatedAt, &lastActive,
	)
	if err != nil {
		return nil, fmt.Errorf("scan learner: %w", err)
	}
	if lastActive.Valid {
		l.LastActive = lastActive.Time
	}
	if profileJSON.Valid {
		l.ProfileJSON = profileJSON.String
	} else {
		l.ProfileJSON = "{}"
	}
	return l, nil
}

func (s *Store) UpdateLastActive(id string) error {
	_, err := s.db.Exec(
		`UPDATE learners SET last_active = ? WHERE id = ?`,
		time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("update last active: %w", err)
	}
	return nil
}

func (s *Store) GetActiveLearners() ([]*models.Learner, error) {
	rows, err := s.db.Query(
		`SELECT id, email, password_hash, objective, webhook_url, profile_json, created_at, last_active
		 FROM learners WHERE webhook_url != ''`,
	)
	if err != nil {
		return nil, fmt.Errorf("get active learners: %w", err)
	}
	defer rows.Close()

	var learners []*models.Learner
	for rows.Next() {
		l := &models.Learner{}
		var lastActive sql.NullTime
		var profileJSON sql.NullString
		if err := rows.Scan(
			&l.ID, &l.Email, &l.PasswordHash, &l.Objective, &l.WebhookURL, &profileJSON, &l.CreatedAt, &lastActive,
		); err != nil {
			return nil, fmt.Errorf("scan learner row: %w", err)
		}
		if lastActive.Valid {
			l.LastActive = lastActive.Time
		}
		if profileJSON.Valid {
			l.ProfileJSON = profileJSON.String
		} else {
			l.ProfileJSON = "{}"
		}
		learners = append(learners, l)
	}
	return learners, rows.Err()
}

func (s *Store) UpdateLearnerProfile(learnerID, profileJSON string) error {
	_, err := s.db.Exec(
		`UPDATE learners SET profile_json = ? WHERE id = ?`,
		profileJSON, learnerID,
	)
	if err != nil {
		return fmt.Errorf("update learner profile: %w", err)
	}
	return nil
}

// ─── Refresh Tokens ───────────────────────────────────────────────────────────

func (s *Store) CreateRefreshToken(learnerID string) (*models.RefreshToken, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}
	token := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(b)
	now := time.Now().UTC()
	expiresAt := now.Add(30 * 24 * time.Hour)

	_, err := s.db.Exec(
		`INSERT INTO refresh_tokens (token, learner_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		token, learnerID, expiresAt, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create refresh token: %w", err)
	}
	return &models.RefreshToken{
		Token:     token,
		LearnerID: learnerID,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}, nil
}

func (s *Store) GetRefreshToken(token string) (*models.RefreshToken, error) {
	rt := &models.RefreshToken{}
	err := s.db.QueryRow(
		`SELECT token, learner_id, expires_at, created_at
		 FROM refresh_tokens WHERE token = ? AND expires_at > ?`,
		token, time.Now().UTC(),
	).Scan(&rt.Token, &rt.LearnerID, &rt.ExpiresAt, &rt.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get refresh token: %w", err)
	}
	return rt, nil
}

func (s *Store) DeleteRefreshToken(token string) error {
	_, err := s.db.Exec(`DELETE FROM refresh_tokens WHERE token = ?`, token)
	if err != nil {
		return fmt.Errorf("delete refresh token: %w", err)
	}
	return nil
}

// ─── Domains ──────────────────────────────────────────────────────────────────

func (s *Store) CreateDomain(learnerID, name, personalGoal string, graph models.KnowledgeSpace) (*models.Domain, error) {
	return s.CreateDomainWithValueFramings(learnerID, name, personalGoal, graph, "")
}

// CreateDomainWithValueFramings creates a domain and optionally persists a JSON-encoded
// set of value framings (4 axes: financial, employment, intellectual, innovation).
func (s *Store) CreateDomainWithValueFramings(learnerID, name, personalGoal string, graph models.KnowledgeSpace, valueFramingsJSON string) (*models.Domain, error) {
	id := generateID()
	now := time.Now().UTC()

	graphJSON, err := json.Marshal(graph)
	if err != nil {
		return nil, fmt.Errorf("marshal graph: %w", err)
	}

	_, err = s.db.Exec(
		`INSERT INTO domains (id, learner_id, name, personal_goal, graph_json, value_framings_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, learnerID, name, personalGoal, string(graphJSON), valueFramingsJSON, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create domain: %w", err)
	}
	return &models.Domain{
		ID:                id,
		LearnerID:         learnerID,
		Name:              name,
		PersonalGoal:      personalGoal,
		Graph:             graph,
		ValueFramingsJSON: valueFramingsJSON,
		CreatedAt:         now,
	}, nil
}

const domainCols = `id, learner_id, name, personal_goal, graph_json, value_framings_json, last_value_axis, archived, created_at`

func scanDomainRow(row *sql.Row) (*models.Domain, error) {
	d := &models.Domain{}
	var graphJSON string
	var valueFramings, lastAxis sql.NullString
	var archived int
	err := row.Scan(&d.ID, &d.LearnerID, &d.Name, &d.PersonalGoal, &graphJSON, &valueFramings, &lastAxis, &archived, &d.CreatedAt)
	if err != nil {
		return nil, err
	}
	d.Archived = archived != 0
	if valueFramings.Valid {
		d.ValueFramingsJSON = valueFramings.String
	}
	if lastAxis.Valid {
		d.LastValueAxis = lastAxis.String
	}
	if err := json.Unmarshal([]byte(graphJSON), &d.Graph); err != nil {
		return nil, fmt.Errorf("unmarshal graph: %w", err)
	}
	return d, nil
}

func scanDomainRows(rows *sql.Rows) (*models.Domain, error) {
	d := &models.Domain{}
	var graphJSON string
	var valueFramings, lastAxis sql.NullString
	var archived int
	err := rows.Scan(&d.ID, &d.LearnerID, &d.Name, &d.PersonalGoal, &graphJSON, &valueFramings, &lastAxis, &archived, &d.CreatedAt)
	if err != nil {
		return nil, err
	}
	d.Archived = archived != 0
	if valueFramings.Valid {
		d.ValueFramingsJSON = valueFramings.String
	}
	if lastAxis.Valid {
		d.LastValueAxis = lastAxis.String
	}
	if err := json.Unmarshal([]byte(graphJSON), &d.Graph); err != nil {
		return nil, fmt.Errorf("unmarshal graph: %w", err)
	}
	return d, nil
}

func (s *Store) GetDomainByLearner(learnerID string) (*models.Domain, error) {
	row := s.db.QueryRow(
		`SELECT `+domainCols+` FROM domains WHERE learner_id = ? AND archived = 0 ORDER BY created_at DESC LIMIT 1`,
		learnerID,
	)
	d, err := scanDomainRow(row)
	if err != nil {
		return nil, fmt.Errorf("get domain by learner: %w", err)
	}
	return d, nil
}

func (s *Store) GetDomainByID(id string) (*models.Domain, error) {
	row := s.db.QueryRow(`SELECT `+domainCols+` FROM domains WHERE id = ?`, id)
	d, err := scanDomainRow(row)
	if err != nil {
		return nil, fmt.Errorf("get domain by id: %w", err)
	}
	return d, nil
}

func (s *Store) GetDomainsByLearner(learnerID string, includeArchived bool) ([]*models.Domain, error) {
	query := `SELECT ` + domainCols + ` FROM domains WHERE learner_id = ?`
	if !includeArchived {
		query += ` AND archived = 0`
	}
	query += ` ORDER BY created_at DESC`
	rows, err := s.db.Query(query, learnerID)
	if err != nil {
		return nil, fmt.Errorf("get domains by learner: %w", err)
	}
	defer rows.Close()

	var domains []*models.Domain
	for rows.Next() {
		d, err := scanDomainRows(rows)
		if err != nil {
			return nil, fmt.Errorf("scan domain row: %w", err)
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}

// UpdateDomainValueFramings stores the JSON-encoded value framings for a domain.
func (s *Store) UpdateDomainValueFramings(domainID, valueFramingsJSON string) error {
	_, err := s.db.Exec(
		`UPDATE domains SET value_framings_json = ? WHERE id = ?`,
		valueFramingsJSON, domainID,
	)
	if err != nil {
		return fmt.Errorf("update domain value framings: %w", err)
	}
	return nil
}

// UpdateDomainLastValueAxis records which axis was surfaced most recently (used for rotation).
func (s *Store) UpdateDomainLastValueAxis(domainID, axis string) error {
	_, err := s.db.Exec(
		`UPDATE domains SET last_value_axis = ? WHERE id = ?`,
		axis, domainID,
	)
	if err != nil {
		return fmt.Errorf("update domain last value axis: %w", err)
	}
	return nil
}

func (s *Store) UpdateDomainGraph(domainID string, graph models.KnowledgeSpace) error {
	graphJSON, err := json.Marshal(graph)
	if err != nil {
		return fmt.Errorf("marshal graph: %w", err)
	}
	_, err = s.db.Exec(
		`UPDATE domains SET graph_json = ? WHERE id = ?`,
		string(graphJSON), domainID,
	)
	if err != nil {
		return fmt.Errorf("update domain graph: %w", err)
	}
	return nil
}

func (s *Store) ArchiveDomain(domainID, learnerID string) error {
	result, err := s.db.Exec(
		`UPDATE domains SET archived = 1 WHERE id = ? AND learner_id = ?`,
		domainID, learnerID,
	)
	if err != nil {
		return fmt.Errorf("archive domain: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("domain not found")
	}
	return nil
}

func (s *Store) UnarchiveDomain(domainID, learnerID string) error {
	result, err := s.db.Exec(
		`UPDATE domains SET archived = 0 WHERE id = ? AND learner_id = ?`,
		domainID, learnerID,
	)
	if err != nil {
		return fmt.Errorf("unarchive domain: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("domain not found")
	}
	return nil
}

// ActiveDomainConceptSet returns the set of concepts that belong to at least
// one non-archived domain owned by the learner. Used by readers to filter out
// orphan concept_states / interactions left behind by delete_domain (which
// intentionally preserves history but removes the domain row).
func (s *Store) ActiveDomainConceptSet(learnerID string) (map[string]bool, error) {
	domains, err := s.GetDomainsByLearner(learnerID, false)
	if err != nil {
		return nil, fmt.Errorf("active domain concept set: %w", err)
	}
	set := make(map[string]bool)
	for _, d := range domains {
		for _, c := range d.Graph.Concepts {
			set[c] = true
		}
	}
	return set, nil
}

func (s *Store) DeleteDomain(domainID, learnerID string) error {
	result, err := s.db.Exec(
		`DELETE FROM domains WHERE id = ? AND learner_id = ?`,
		domainID, learnerID,
	)
	if err != nil {
		return fmt.Errorf("delete domain: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("domain not found")
	}
	return nil
}

func (s *Store) InsertConceptStateIfNotExists(cs *models.ConceptState) error {
	cs.UpdatedAt = time.Now().UTC()
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO concept_states
		    (learner_id, concept, stability, difficulty, elapsed_days, scheduled_days,
		     reps, lapses, card_state, last_review, next_review, p_mastery, p_learn, p_forget,
		     p_slip, p_guess, theta, pfa_successes, pfa_failures, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cs.LearnerID, cs.Concept, cs.Stability, cs.Difficulty, cs.ElapsedDays, cs.ScheduledDays,
		cs.Reps, cs.Lapses, cs.CardState, cs.LastReview, cs.NextReview,
		cs.PMastery, cs.PLearn, cs.PForget, cs.PSlip, cs.PGuess,
		cs.Theta, cs.PFASuccesses, cs.PFAFailures, cs.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert concept state if not exists: %w", err)
	}
	return nil
}

// ─── Concept States ───────────────────────────────────────────────────────────

func (s *Store) GetConceptState(learnerID, concept string) (*models.ConceptState, error) {
	cs := &models.ConceptState{}
	var lastReview, nextReview sql.NullTime
	err := s.db.QueryRow(
		`SELECT id, learner_id, concept, stability, difficulty, elapsed_days, scheduled_days,
		        reps, lapses, card_state, last_review, next_review, p_mastery, p_learn, p_forget,
		        p_slip, p_guess, theta, pfa_successes, pfa_failures, updated_at
		 FROM concept_states WHERE learner_id = ? AND concept = ?`,
		learnerID, concept,
	).Scan(
		&cs.ID, &cs.LearnerID, &cs.Concept, &cs.Stability, &cs.Difficulty,
		&cs.ElapsedDays, &cs.ScheduledDays, &cs.Reps, &cs.Lapses, &cs.CardState,
		&lastReview, &nextReview,
		&cs.PMastery, &cs.PLearn, &cs.PForget, &cs.PSlip, &cs.PGuess,
		&cs.Theta, &cs.PFASuccesses, &cs.PFAFailures, &cs.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get concept state: %w", err)
	}
	if lastReview.Valid {
		cs.LastReview = &lastReview.Time
	}
	if nextReview.Valid {
		cs.NextReview = &nextReview.Time
	}
	return cs, nil
}

func (s *Store) UpsertConceptState(cs *models.ConceptState) error {
	cs.UpdatedAt = time.Now().UTC()
	_, err := s.db.Exec(
		`INSERT INTO concept_states
		    (learner_id, concept, stability, difficulty, elapsed_days, scheduled_days,
		     reps, lapses, card_state, last_review, next_review, p_mastery, p_learn, p_forget,
		     p_slip, p_guess, theta, pfa_successes, pfa_failures, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(learner_id, concept) DO UPDATE SET
		    stability      = excluded.stability,
		    difficulty     = excluded.difficulty,
		    elapsed_days   = excluded.elapsed_days,
		    scheduled_days = excluded.scheduled_days,
		    reps           = excluded.reps,
		    lapses         = excluded.lapses,
		    card_state     = excluded.card_state,
		    last_review    = excluded.last_review,
		    next_review    = excluded.next_review,
		    p_mastery      = excluded.p_mastery,
		    p_learn        = excluded.p_learn,
		    p_forget       = excluded.p_forget,
		    p_slip         = excluded.p_slip,
		    p_guess        = excluded.p_guess,
		    theta          = excluded.theta,
		    pfa_successes  = excluded.pfa_successes,
		    pfa_failures   = excluded.pfa_failures,
		    updated_at     = excluded.updated_at`,
		cs.LearnerID, cs.Concept, cs.Stability, cs.Difficulty, cs.ElapsedDays, cs.ScheduledDays,
		cs.Reps, cs.Lapses, cs.CardState, cs.LastReview, cs.NextReview,
		cs.PMastery, cs.PLearn, cs.PForget, cs.PSlip, cs.PGuess,
		cs.Theta, cs.PFASuccesses, cs.PFAFailures, cs.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert concept state: %w", err)
	}
	return nil
}

func (s *Store) GetConceptStatesByLearner(learnerID string) ([]*models.ConceptState, error) {
	rows, err := s.db.Query(
		`SELECT id, learner_id, concept, stability, difficulty, elapsed_days, scheduled_days,
		        reps, lapses, card_state, last_review, next_review, p_mastery, p_learn, p_forget,
		        p_slip, p_guess, theta, pfa_successes, pfa_failures, updated_at
		 FROM concept_states WHERE learner_id = ?`,
		learnerID,
	)
	if err != nil {
		return nil, fmt.Errorf("get concept states by learner: %w", err)
	}
	defer rows.Close()

	var states []*models.ConceptState
	for rows.Next() {
		cs := &models.ConceptState{}
		var lastReview, nextReview sql.NullTime
		if err := rows.Scan(
			&cs.ID, &cs.LearnerID, &cs.Concept, &cs.Stability, &cs.Difficulty,
			&cs.ElapsedDays, &cs.ScheduledDays, &cs.Reps, &cs.Lapses, &cs.CardState,
			&lastReview, &nextReview,
			&cs.PMastery, &cs.PLearn, &cs.PForget, &cs.PSlip, &cs.PGuess,
			&cs.Theta, &cs.PFASuccesses, &cs.PFAFailures, &cs.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan concept state row: %w", err)
		}
		if lastReview.Valid {
			cs.LastReview = &lastReview.Time
		}
		if nextReview.Valid {
			cs.NextReview = &nextReview.Time
		}
		states = append(states, cs)
	}
	return states, rows.Err()
}

// ─── Interactions ─────────────────────────────────────────────────────────────

const interactionCols = `id, learner_id, concept, activity_type, success, response_time, confidence, error_type, notes, hints_requested, self_initiated, calibration_id, is_proactive_review, misconception_type, misconception_detail, created_at`

func (s *Store) CreateInteraction(i *models.Interaction) error {
	i.CreatedAt = time.Now().UTC()
	result, err := s.db.Exec(
		`INSERT INTO interactions (learner_id, concept, activity_type, success, response_time, confidence, error_type, notes, hints_requested, self_initiated, calibration_id, is_proactive_review, misconception_type, misconception_detail, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		i.LearnerID, i.Concept, i.ActivityType, boolToInt(i.Success),
		i.ResponseTime, i.Confidence, i.ErrorType, i.Notes,
		i.HintsRequested, boolToInt(i.SelfInitiated), i.CalibrationID, boolToInt(i.IsProactiveReview),
		nullString(i.MisconceptionType), nullString(i.MisconceptionDetail),
		i.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create interaction: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get interaction id: %w", err)
	}
	i.ID = id
	return nil
}

func (s *Store) GetRecentInteractions(learnerID, concept string, limit int) ([]*models.Interaction, error) {
	rows, err := s.db.Query(
		`SELECT `+interactionCols+` FROM interactions WHERE learner_id = ? AND concept = ?
		 ORDER BY created_at DESC LIMIT ?`,
		learnerID, concept, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get recent interactions: %w", err)
	}
	defer rows.Close()
	return scanInteractions(rows)
}

func (s *Store) GetRecentInteractionsByLearner(learnerID string, limit int) ([]*models.Interaction, error) {
	rows, err := s.db.Query(
		`SELECT `+interactionCols+` FROM interactions WHERE learner_id = ?
		 ORDER BY created_at DESC LIMIT ?`,
		learnerID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get recent interactions by learner: %w", err)
	}
	defer rows.Close()
	return scanInteractions(rows)
}

func (s *Store) GetSessionInteractions(learnerID string) ([]*models.Interaction, error) {
	cutoff := time.Now().UTC().Add(-2 * time.Hour)
	rows, err := s.db.Query(
		`SELECT `+interactionCols+` FROM interactions WHERE learner_id = ? AND created_at > ?
		 ORDER BY created_at DESC`,
		learnerID, cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("get session interactions: %w", err)
	}
	defer rows.Close()
	return scanInteractions(rows)
}

func scanInteractions(rows *sql.Rows) ([]*models.Interaction, error) {
	var interactions []*models.Interaction
	for rows.Next() {
		i := &models.Interaction{}
		var successInt, selfInitInt, proactiveInt int
		var errorType, calibrationID, misconceptionType, misconceptionDetail sql.NullString
		if err := rows.Scan(
			&i.ID, &i.LearnerID, &i.Concept, &i.ActivityType,
			&successInt, &i.ResponseTime, &i.Confidence, &errorType, &i.Notes,
			&i.HintsRequested, &selfInitInt, &calibrationID, &proactiveInt,
			&misconceptionType, &misconceptionDetail,
			&i.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan interaction row: %w", err)
		}
		i.Success = successInt != 0
		i.SelfInitiated = selfInitInt != 0
		i.IsProactiveReview = proactiveInt != 0
		if errorType.Valid {
			i.ErrorType = errorType.String
		}
		if calibrationID.Valid {
			i.CalibrationID = calibrationID.String
		}
		if misconceptionType.Valid {
			i.MisconceptionType = misconceptionType.String
		}
		if misconceptionDetail.Valid {
			i.MisconceptionDetail = misconceptionDetail.String
		}
		interactions = append(interactions, i)
	}
	return interactions, rows.Err()
}

func (s *Store) GetInteractionsSince(learnerID string, since time.Time) ([]*models.Interaction, error) {
	rows, err := s.db.Query(
		`SELECT `+interactionCols+` FROM interactions WHERE learner_id = ? AND created_at >= ? ORDER BY created_at ASC`,
		learnerID, since,
	)
	if err != nil {
		return nil, fmt.Errorf("get interactions since: %w", err)
	}
	defer rows.Close()
	return scanInteractions(rows)
}

func (s *Store) GetSessionStart(learnerID string) (time.Time, error) {
	cutoff := time.Now().UTC().Add(-2 * time.Hour)
	var sessionStart sql.NullTime
	err := s.db.QueryRow(
		`SELECT MIN(created_at) FROM interactions WHERE learner_id = ? AND created_at > ?`,
		learnerID, cutoff,
	).Scan(&sessionStart)
	if err != nil {
		return time.Time{}, fmt.Errorf("get session start: %w", err)
	}
	if !sessionStart.Valid {
		return time.Now().UTC(), nil
	}
	return sessionStart.Time, nil
}

// ─── Availability ─────────────────────────────────────────────────────────────

func (s *Store) GetAvailability(learnerID string) (*models.Availability, error) {
	a := &models.Availability{}
	var dndInt int
	err := s.db.QueryRow(
		`SELECT learner_id, windows_json, avg_duration, sessions_week, do_not_disturb
		 FROM availability WHERE learner_id = ?`,
		learnerID,
	).Scan(&a.LearnerID, &a.WindowsJSON, &a.AvgDuration, &a.SessionsWeek, &dndInt)
	if err == sql.ErrNoRows {
		return &models.Availability{
			LearnerID:    learnerID,
			WindowsJSON:  "[]",
			AvgDuration:  30,
			SessionsWeek: 3,
			DoNotDisturb: false,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get availability: %w", err)
	}
	a.DoNotDisturb = dndInt != 0
	return a, nil
}

func (s *Store) UpsertAvailability(a *models.Availability) error {
	_, err := s.db.Exec(
		`INSERT INTO availability (learner_id, windows_json, avg_duration, sessions_week, do_not_disturb)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(learner_id) DO UPDATE SET
		    windows_json   = excluded.windows_json,
		    avg_duration   = excluded.avg_duration,
		    sessions_week  = excluded.sessions_week,
		    do_not_disturb = excluded.do_not_disturb`,
		a.LearnerID, a.WindowsJSON, a.AvgDuration, a.SessionsWeek, boolToInt(a.DoNotDisturb),
	)
	if err != nil {
		return fmt.Errorf("upsert availability: %w", err)
	}
	return nil
}

// ─── Scheduled Alerts ─────────────────────────────────────────────────────────

func (s *Store) CreateScheduledAlert(learnerID, alertType, concept string, scheduledAt time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO scheduled_alerts (learner_id, alert_type, concept, scheduled_at) VALUES (?, ?, ?, ?)`,
		learnerID, alertType, concept, scheduledAt,
	)
	if err != nil {
		return fmt.Errorf("create scheduled alert: %w", err)
	}
	return nil
}

func (s *Store) GetUnsentAlerts(learnerID string) ([]*models.ScheduledAlert, error) {
	rows, err := s.db.Query(
		`SELECT id, learner_id, alert_type, concept, scheduled_at, sent, created_at
		 FROM scheduled_alerts WHERE learner_id = ? AND sent = 0`,
		learnerID,
	)
	if err != nil {
		return nil, fmt.Errorf("get unsent alerts: %w", err)
	}
	defer rows.Close()

	var alerts []*models.ScheduledAlert
	for rows.Next() {
		sa := &models.ScheduledAlert{}
		var sentInt int
		if err := rows.Scan(
			&sa.ID, &sa.LearnerID, &sa.AlertType, &sa.Concept,
			&sa.ScheduledAt, &sentInt, &sa.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan alert row: %w", err)
		}
		sa.Sent = sentInt != 0
		alerts = append(alerts, sa)
	}
	return alerts, rows.Err()
}

func (s *Store) MarkAlertSent(id int64) error {
	_, err := s.db.Exec(`UPDATE scheduled_alerts SET sent = 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("mark alert sent: %w", err)
	}
	return nil
}

func (s *Store) WasAlertSentToday(learnerID, alertType string) (bool, error) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM scheduled_alerts
		 WHERE learner_id = ? AND alert_type = ? AND created_at >= ?`,
		learnerID, alertType, today,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check alert sent today: %w", err)
	}
	return count > 0, nil
}

// ─── Stats for Scheduler ─────────────────────────────────────────────────────

// GetDailyStreak returns how many consecutive days the learner has had interactions.
func (s *Store) GetDailyStreak(learnerID string) (int, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT DATE(created_at) as d FROM interactions
		 WHERE learner_id = ? ORDER BY d DESC`,
		learnerID,
	)
	if err != nil {
		return 0, fmt.Errorf("get daily streak: %w", err)
	}
	defer rows.Close()

	streak := 0
	expected := time.Now().UTC().Truncate(24 * time.Hour)
	for rows.Next() {
		var dateStr string
		if err := rows.Scan(&dateStr); err != nil {
			return streak, nil
		}
		d, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return streak, nil
		}
		// Allow today or yesterday to start the streak
		if streak == 0 {
			diff := expected.Sub(d).Hours() / 24
			if diff > 1 {
				return 0, nil // last activity was more than 1 day ago
			}
			streak = 1
			expected = d.AddDate(0, 0, -1)
			continue
		}
		if d.Equal(expected) {
			streak++
			expected = d.AddDate(0, 0, -1)
		} else {
			break
		}
	}
	return streak, nil
}

// GetTodayInteractionCount returns the number of interactions today.
func (s *Store) GetTodayInteractionCount(learnerID string) (int, error) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM interactions WHERE learner_id = ? AND created_at >= ?`,
		learnerID, today,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("get today interactions: %w", err)
	}
	return count, nil
}

// GetTodaySuccessRate returns success rate for today's interactions.
func (s *Store) GetTodaySuccessRate(learnerID string) (float64, int, error) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	var total, successes int
	err := s.db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(success), 0) FROM interactions
		 WHERE learner_id = ? AND created_at >= ?`,
		learnerID, today,
	).Scan(&total, &successes)
	if err != nil {
		return 0, 0, fmt.Errorf("get today success rate: %w", err)
	}
	if total == 0 {
		return 0, 0, nil
	}
	return float64(successes) / float64(total), total, nil
}

// GetConceptsDueForReview returns concepts where next_review is in the past.
func (s *Store) GetConceptsDueForReview(learnerID string) ([]string, error) {
	now := time.Now().UTC()
	rows, err := s.db.Query(
		`SELECT concept FROM concept_states
		 WHERE learner_id = ? AND next_review IS NOT NULL AND next_review <= ? AND card_state != 'new'
		 ORDER BY next_review ASC`,
		learnerID, now,
	)
	if err != nil {
		return nil, fmt.Errorf("get concepts due for review: %w", err)
	}
	defer rows.Close()

	var concepts []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return concepts, nil
		}
		concepts = append(concepts, c)
	}
	return concepts, nil
}

// ─── OAuth Persistence ──────────────────────────────────────────────────────

// AuthCode holds the authorization code state (persisted in DB).
type AuthCode struct {
	Code          string
	LearnerID     string
	CodeChallenge string
	ClientID      string
	ExpiresAt     time.Time
}

// OAuthClient is a dynamically-registered OAuth client.
// RedirectURIs holds the JSON array as persisted.
// ClientSecretHash is hex-encoded SHA-256 of the secret; empty for public (PKCE-only) clients.
type OAuthClient struct {
	ClientID         string
	ClientName       string
	RedirectURIs     string
	ClientSecretHash string
}

func (s *Store) CreateAuthCode(code, learnerID, codeChallenge, clientID string, expiresAt time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO oauth_codes (code, learner_id, code_challenge, client_id, expires_at) VALUES (?, ?, ?, ?, ?)`,
		code, learnerID, codeChallenge, clientID, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("create auth code: %w", err)
	}
	return nil
}

// ConsumeAuthCode retrieves and deletes an auth code in one operation.
// Binds the code to the requesting client_id: returns invalid_grant if mismatch.
func (s *Store) ConsumeAuthCode(code, clientID string) (*AuthCode, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	ac := &AuthCode{}
	err = tx.QueryRow(
		`SELECT code, learner_id, code_challenge, client_id, expires_at FROM oauth_codes WHERE code = ? AND client_id = ?`,
		code, clientID,
	).Scan(&ac.Code, &ac.LearnerID, &ac.CodeChallenge, &ac.ClientID, &ac.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("invalid_grant")
	}
	if err != nil {
		return nil, fmt.Errorf("consume auth code: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM oauth_codes WHERE code = ?`, code); err != nil {
		return nil, fmt.Errorf("delete auth code: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit consume auth code: %w", err)
	}
	return ac, nil
}

func (s *Store) CreateOAuthClient(clientID, clientName, redirectURIs string) error {
	return s.CreateOAuthClientWithSecret(clientID, clientName, redirectURIs, "")
}

// CreateOAuthClientWithSecret persists a confidential client when secretHash != "".
// secretHash should be hex-encoded SHA-256 of the issued secret; pass "" for public (PKCE) clients.
func (s *Store) CreateOAuthClientWithSecret(clientID, clientName, redirectURIs, secretHash string) error {
	_, err := s.db.Exec(
		`INSERT INTO oauth_clients (client_id, client_name, redirect_uris, client_secret_hash) VALUES (?, ?, ?, ?)`,
		clientID, clientName, redirectURIs, secretHash,
	)
	if err != nil {
		return fmt.Errorf("create oauth client: %w", err)
	}
	return nil
}

func (s *Store) GetOAuthClient(clientID string) (*OAuthClient, error) {
	c := &OAuthClient{}
	var secretHash sql.NullString
	err := s.db.QueryRow(
		`SELECT client_id, client_name, redirect_uris, client_secret_hash FROM oauth_clients WHERE client_id = ?`,
		clientID,
	).Scan(&c.ClientID, &c.ClientName, &c.RedirectURIs, &secretHash)
	if err != nil {
		return nil, fmt.Errorf("get oauth client: %w", err)
	}
	if secretHash.Valid {
		c.ClientSecretHash = secretHash.String
	}
	return c, nil
}

func (s *Store) CleanupExpiredCodes() (int64, error) {
	result, err := s.db.Exec(`DELETE FROM oauth_codes WHERE expires_at < ?`, time.Now().UTC())
	if err != nil {
		return 0, fmt.Errorf("cleanup expired codes: %w", err)
	}
	return result.RowsAffected()
}

func (s *Store) CleanupExpiredRefreshTokens() (int64, error) {
	result, err := s.db.Exec(`DELETE FROM refresh_tokens WHERE expires_at < ?`, time.Now().UTC())
	if err != nil {
		return 0, fmt.Errorf("cleanup expired refresh tokens: %w", err)
	}
	return result.RowsAffected()
}
