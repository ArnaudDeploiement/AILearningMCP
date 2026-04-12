package models

import "time"

type AlertType string

const (
	AlertForgetting   AlertType = "FORGETTING"
	AlertPlateau      AlertType = "PLATEAU"
	AlertZPDDrift     AlertType = "ZPD_DRIFT"
	AlertOverload     AlertType = "OVERLOAD"
	AlertMasteryReady            AlertType = "MASTERY_READY"
	AlertDependencyIncreasing    AlertType = "DEPENDENCY_INCREASING"
	AlertCalibrationDiverging    AlertType = "CALIBRATION_DIVERGING"
	AlertAffectNegative          AlertType = "AFFECT_NEGATIVE"
	AlertTransferBlocked         AlertType = "TRANSFER_BLOCKED"
)

type AlertUrgency string

const (
	UrgencyCritical AlertUrgency = "critical"
	UrgencyWarning  AlertUrgency = "warning"
	UrgencyInfo     AlertUrgency = "info"
)

type Alert struct {
	Type               AlertType    `json:"type"`
	Concept            string       `json:"concept"`
	Urgency            AlertUrgency `json:"urgency"`
	Retention          float64      `json:"retention,omitempty"`
	HoursUntilCritical float64      `json:"hours_until_critical,omitempty"`
	SessionsStalled    int          `json:"sessions_stalled,omitempty"`
	ErrorRate          float64      `json:"error_rate,omitempty"`
	RecommendedAction  string       `json:"recommended_action"`
}

type ActivityType string

const (
	ActivityRecall           ActivityType = "RECALL_EXERCISE"
	ActivityNewConcept       ActivityType = "NEW_CONCEPT"
	ActivityInterleaving     ActivityType = "INTERLEAVING"
	ActivityMasteryChallenge ActivityType = "MASTERY_CHALLENGE"
	ActivityDebuggingCase    ActivityType = "DEBUGGING_CASE"
	ActivityRest             ActivityType = "REST"
	ActivitySetupDomain      ActivityType = "SETUP_DOMAIN"
)

type Activity struct {
	Type             ActivityType `json:"type"`
	Concept          string       `json:"concept"`
	DifficultyTarget float64      `json:"difficulty_target"`
	Format           string       `json:"format"`
	EstimatedMinutes int          `json:"estimated_minutes"`
	Rationale        string       `json:"rationale"`
	PromptForLLM     string       `json:"prompt_for_llm"`
}

type KnowledgeSpace struct {
	Concepts      []string            `json:"concepts"`
	Prerequisites map[string][]string `json:"prerequisites"`
}

type Domain struct {
	ID                 string
	LearnerID          string
	Name               string
	PersonalGoal       string
	Graph              KnowledgeSpace
	ValueFramingsJSON  string
	LastValueAxis      string
	Archived           bool
	CreatedAt          time.Time
}

type TimeWindow struct {
	Day   string `json:"day"`
	Start string `json:"start"`
	End   string `json:"end"`
}
