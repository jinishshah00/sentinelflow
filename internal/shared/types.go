package shared

import "time"

type Severity string

const (
	SeverityLow    Severity = "low"
	SeverityMedium Severity = "medium"
	SeverityHigh   Severity = "high"
)

type Event struct {
	ID           string    `json:"id"`
	EventType    string    `json:"event_type"`
	Principal    string    `json:"principal"`
	Target       string    `json:"target"`
	Network      string    `json:"network"`
	SeverityHint string    `json:"severity_hint"`
	Labels       []string  `json:"labels"`
	Description  string    `json:"description"`
	TS           time.Time `json:"ts"`
}

// LabeledEvent is used only for training/evaluation datasets.
type LabeledEvent struct {
	Event
	Y Severity `json:"y"` // ground-truth label: low | medium | high
}
