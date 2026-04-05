package models

import "time"

type Learner struct {
	ID           string
	Email        string
	PasswordHash string
	Objective    string
	WebhookURL   string
	ProfileJSON  string
	CreatedAt    time.Time
	LastActive   time.Time
}

type ConceptState struct {
	ID            int64
	LearnerID     string
	Concept       string
	Stability     float64
	Difficulty    float64
	ElapsedDays   int
	ScheduledDays int
	Reps          int
	Lapses        int
	CardState     string
	LastReview    *time.Time
	NextReview    *time.Time
	PMastery      float64
	PLearn        float64
	PForget       float64
	PSlip         float64
	PGuess        float64
	Theta         float64
	PFASuccesses  float64
	PFAFailures   float64
	UpdatedAt     time.Time
}

func NewConceptState(learnerID, concept string) *ConceptState {
	return &ConceptState{
		LearnerID:  learnerID,
		Concept:    concept,
		Stability:  1.0,
		Difficulty: 0.3,
		CardState:  "new",
		PMastery:   0.1,
		PLearn:     0.3,
		PForget:    0.05,
		PSlip:      0.1,
		PGuess:     0.2,
		Theta:      0.0,
	}
}

type Interaction struct {
	ID           int64
	LearnerID    string
	Concept      string
	ActivityType string
	Success      bool
	ResponseTime int
	Confidence   float64
	ErrorType         string
	Notes             string
	HintsRequested    int
	SelfInitiated     bool
	CalibrationID     string
	IsProactiveReview bool
	CreatedAt         time.Time
}

type RefreshToken struct {
	Token     string
	LearnerID string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type Availability struct {
	LearnerID    string
	WindowsJSON  string
	AvgDuration  int
	SessionsWeek int
	DoNotDisturb bool
}

type ScheduledAlert struct {
	ID          int64
	LearnerID   string
	AlertType   string
	Concept     string
	ScheduledAt time.Time
	Sent        bool
	CreatedAt   time.Time
}
