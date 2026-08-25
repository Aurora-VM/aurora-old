package health

import (
	"context"
	"testing"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/health"
	"github.com/stretchr/testify/assert"
)

type mockChecker struct {
	name   string
	status health.Status
}

func (m *mockChecker) Name() string { return m.name }
func (m *mockChecker) Check() health.ComponentStatus {
	return health.ComponentStatus{
		Name:      m.name,
		Status:    m.status,
		CheckedAt: time.Now(),
	}
}

func TestHealthService_GetHealth_Healthy(t *testing.T) {
	svc := NewService(&mockChecker{name: "db", status: health.StatusHealthy})
	h := svc.GetHealth(context.Background())
	assert.Equal(t, health.StatusHealthy, h.Status)
	assert.Contains(t, h.Components, "db")
}

func TestHealthService_GetHealth_Degraded(t *testing.T) {
	svc := NewService(
		&mockChecker{name: "db", status: health.StatusHealthy},
		&mockChecker{name: "redis", status: health.StatusDegraded},
	)
	h := svc.GetHealth(context.Background())
	assert.Equal(t, health.StatusDegraded, h.Status)
}
