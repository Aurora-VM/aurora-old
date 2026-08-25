package health

import (
	"context"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/health"
	"github.com/aurora-vm/aurora/pkg/version"
)

// Service coordinates health checks across Aurora subsystems.
type Service struct {
	startTime         time.Time
	livenessCheckers  []health.Checker
	readinessCheckers []health.Checker
}

// NewService creates a new health application service.
func NewService(readinessCheckers ...health.Checker) *Service {
	return &Service{
		startTime:         time.Now(),
		livenessCheckers:  nil,
		readinessCheckers: readinessCheckers,
	}
}

// CheckLiveness performs a lightweight liveness verification.
func (s *Service) CheckLiveness(ctx context.Context) bool {
	return true
}

// CheckReadiness checks if all required dependencies are ready to accept traffic.
func (s *Service) CheckReadiness(ctx context.Context) (bool, map[string]health.ComponentStatus) {
	components := make(map[string]health.ComponentStatus)
	isReady := true

	for _, checker := range s.readinessCheckers {
		cs := checker.Check()
		components[checker.Name()] = cs
		if cs.Status == health.StatusUnhealthy {
			isReady = false
		}
	}

	return isReady, components
}

// GetHealth collects component statuses and determines overall system health.
func (s *Service) GetHealth(ctx context.Context) health.SystemHealth {
	vInfo := version.Get()
	components := make(map[string]health.ComponentStatus)
	overallStatus := health.StatusHealthy

	for _, checker := range s.readinessCheckers {
		cs := checker.Check()
		components[checker.Name()] = cs

		if cs.Status == health.StatusUnhealthy {
			overallStatus = health.StatusUnhealthy
		} else if cs.Status == health.StatusDegraded && overallStatus != health.StatusUnhealthy {
			overallStatus = health.StatusDegraded
		}
	}

	return health.SystemHealth{
		Status:     overallStatus,
		Version:    vInfo.Version,
		Commit:     vInfo.GitCommit,
		Uptime:     time.Since(s.startTime).Round(time.Second).String(),
		Components: components,
		Timestamp:  time.Now().UTC(),
	}
}
