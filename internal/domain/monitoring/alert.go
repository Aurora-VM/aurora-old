package monitoring

import "time"

type AlertState string

const (
	AlertStateFiring       AlertState = "firing"
	AlertStateAcknowledged AlertState = "acknowledged"
	AlertStateResolved     AlertState = "resolved"
)

// AlertEvent represents an incident or violation triggered by an AlertThreshold.
type AlertEvent struct {
	ID             string        `json:"id"`
	ThresholdID    string        `json:"thresholdId"`
	ResourceType   ResourceType  `json:"resourceType"`
	ResourceID     string        `json:"resourceId"`
	TriggeredValue float64       `json:"triggeredValue"`
	Severity       AlertSeverity `json:"severity"`
	Message        string        `json:"message"`
	State          AlertState    `json:"state"`
	TriggeredAt    time.Time     `json:"triggeredAt"`
	ResolvedAt     *time.Time    `json:"resolvedAt,omitempty"`
}
