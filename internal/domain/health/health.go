package health

import "time"

// Status represents the overall health status of a component or system.
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusDegraded  Status = "degraded"
	StatusUnhealthy Status = "unhealthy"
)

// ComponentStatus represents the status of a specific internal subsystem.
type ComponentStatus struct {
	Name      string    `json:"name"`
	Status    Status    `json:"status"`
	Message   string    `json:"message,omitempty"`
	CheckedAt time.Time `json:"checkedAt"`
}

// SystemHealth encapsulates the overall health report.
type SystemHealth struct {
	Status     Status                     `json:"status"`
	Version    string                     `json:"version"`
	Commit     string                     `json:"commit"`
	Uptime     string                     `json:"uptime"`
	Components map[string]ComponentStatus `json:"components"`
	Timestamp  time.Time                  `json:"timestamp"`
}

// Checker is the interface implemented by infrastructure components to check health.
type Checker interface {
	Name() string
	Check() ComponentStatus
}
