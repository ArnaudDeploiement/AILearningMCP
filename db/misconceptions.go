package db

import (
	"database/sql"
	"fmt"
	"time"
)

// parseTimeFlex attempts several common SQLite/Go time formats.
func parseTimeFlex(s string) (time.Time, error) {
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999 +0000 UTC",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02T15:04:05.999999999-07:00",
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time %q", s)
}

// ─── Misconception Groups ──────────────────────────────────────────────────

type MisconceptionGroup struct {
	Concept           string    `json:"concept"`
	MisconceptionType string    `json:"misconception_type"`
	Count             int       `json:"count"`
	FirstSeen         time.Time `json:"first_seen"`
	LastSeen          time.Time `json:"last_seen"`
	LastErrorDetail   string    `json:"last_error_detail"`
	Status            string    `json:"status"`
}

// GetMisconceptionGroups returns all misconception groups for a learner,
// optionally filtered by concept. Groups are ordered by count descending.
func (s *Store) GetMisconceptionGroups(learnerID string, conceptFilter map[string]bool) ([]MisconceptionGroup, error) {
	rows, err := s.db.Query(
		`SELECT concept, misconception_type, COUNT(*) AS cnt, MIN(created_at), MAX(created_at)
		 FROM interactions
		 WHERE learner_id = ? AND misconception_type IS NOT NULL
		 GROUP BY concept, misconception_type
		 ORDER BY cnt DESC`,
		learnerID,
	)
	if err != nil {
		return nil, fmt.Errorf("get misconception groups: %w", err)
	}
	defer rows.Close()

	var groups []MisconceptionGroup
	for rows.Next() {
		var g MisconceptionGroup
		var firstSeen, lastSeen string
		if err := rows.Scan(&g.Concept, &g.MisconceptionType, &g.Count, &firstSeen, &lastSeen); err != nil {
			return nil, fmt.Errorf("scan misconception group: %w", err)
		}
		g.FirstSeen, _ = parseTimeFlex(firstSeen)
		g.LastSeen, _ = parseTimeFlex(lastSeen)

		// Go-side concept filtering
		if conceptFilter != nil && !conceptFilter[g.Concept] {
			continue
		}

		// Enrich: last_error_detail
		detail, err := s.getLastMisconceptionDetail(learnerID, g.Concept, g.MisconceptionType)
		if err != nil {
			return nil, fmt.Errorf("get last misconception detail: %w", err)
		}
		g.LastErrorDetail = detail

		// Enrich: status
		g.Status = s.computeMisconceptionStatus(learnerID, g.Concept, g.MisconceptionType)

		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// getLastMisconceptionDetail returns the most recent misconception_detail
// for a given (learner, concept, misconception_type) tuple.
func (s *Store) getLastMisconceptionDetail(learnerID, concept, misconceptionType string) (string, error) {
	var detail sql.NullString
	err := s.db.QueryRow(
		`SELECT misconception_detail FROM interactions
		 WHERE learner_id = ? AND concept = ? AND misconception_type = ?
		 ORDER BY created_at DESC LIMIT 1`,
		learnerID, concept, misconceptionType,
	).Scan(&detail)
	if err != nil && err != sql.ErrNoRows {
		return "", fmt.Errorf("query last misconception detail: %w", err)
	}
	if detail.Valid {
		return detail.String, nil
	}
	return "", nil
}

// computeMisconceptionStatus checks the 3 most recent interactions on a concept
// and returns "active" if any of them carry the given misconception_type, or
// "resolved" otherwise.
func (s *Store) computeMisconceptionStatus(learnerID, concept, misconceptionType string) string {
	rows, err := s.db.Query(
		`SELECT misconception_type FROM interactions
		 WHERE learner_id = ? AND concept = ?
		 ORDER BY created_at DESC LIMIT 3`,
		learnerID, concept,
	)
	if err != nil {
		return "active" // err on the side of caution
	}
	defer rows.Close()

	for rows.Next() {
		var mt sql.NullString
		if err := rows.Scan(&mt); err != nil {
			return "active"
		}
		if mt.Valid && mt.String == misconceptionType {
			return "active"
		}
	}
	return "resolved"
}

// GetDistinctMisconceptionTypes returns all distinct misconception types
// recorded for a learner on a given concept, in alphabetical order.
func (s *Store) GetDistinctMisconceptionTypes(learnerID, concept string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT misconception_type FROM interactions
		 WHERE learner_id = ? AND concept = ? AND misconception_type IS NOT NULL
		 ORDER BY misconception_type`,
		learnerID, concept,
	)
	if err != nil {
		return nil, fmt.Errorf("get distinct misconception types: %w", err)
	}
	defer rows.Close()

	var types []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("scan misconception type: %w", err)
		}
		types = append(types, t)
	}
	return types, rows.Err()
}

// GetActiveMisconceptions returns only the "active" misconception groups
// for a learner on a specific concept.
func (s *Store) GetActiveMisconceptions(learnerID, concept string) ([]MisconceptionGroup, error) {
	filter := map[string]bool{concept: true}
	groups, err := s.GetMisconceptionGroups(learnerID, filter)
	if err != nil {
		return nil, fmt.Errorf("get active misconceptions: %w", err)
	}

	var active []MisconceptionGroup
	for _, g := range groups {
		if g.Status == "active" {
			active = append(active, g)
		}
	}
	return active, nil
}
