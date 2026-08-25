package monitoring

import (
	"time"

	"github.com/aurora-vm/aurora/internal/domain/identity"
)

type ComparisonOperator string

const (
	OpGT  ComparisonOperator = "gt"
	OpGTE ComparisonOperator = "gte"
	OpLT  ComparisonOperator = "lt"
	OpLTE ComparisonOperator = "lte"
	OpEQ  ComparisonOperator = "eq"
)

type AlertSeverity string

const (
	SeverityInfo     AlertSeverity = "info"
	SeverityWarning  AlertSeverity = "warning"
	SeverityCritical AlertSeverity = "critical"
)

// AlertThreshold defines an automated condition that fires alerts when violated.
type AlertThreshold struct {
	ID              string             `json:"id"`
	UserID          string             `json:"userId"`
	ResourceType    ResourceType       `json:"resourceType"`
	ResourceID      string             `json:"resourceId"`
	MetricName      string             `json:"metricName"`
	Operator        ComparisonOperator `json:"operator"`
	ThresholdValue  float64            `json:"thresholdValue"`
	DurationSeconds int                `json:"durationSeconds"`
	Severity        AlertSeverity      `json:"severity"`
	Enabled         bool               `json:"enabled"`
	CreatedAt       time.Time          `json:"createdAt"`
	UpdatedAt       time.Time          `json:"updatedAt"`
}

func (t *AlertThreshold) Evaluate(currentValue float64) bool {
	if !t.Enabled {
		return false
	}
	switch t.Operator {
	case OpGT:
		return currentValue > t.ThresholdValue
	case OpGTE:
		return currentValue >= t.ThresholdValue
	case OpLT:
		return currentValue < t.ThresholdValue
	case OpLTE:
		return currentValue <= t.ThresholdValue
	case OpEQ:
		return currentValue == t.ThresholdValue
	default:
		return false
	}
}

func (t *AlertThreshold) Resource() *identity.Resource {
	return &identity.Resource{
		Type:    "alert_threshold",
		ID:      t.ID,
		OwnerID: t.UserID,
	}
}
