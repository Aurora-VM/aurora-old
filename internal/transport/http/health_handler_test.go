package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aurora-vm/aurora/internal/app/health"
	domainHealth "github.com/aurora-vm/aurora/internal/domain/health"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockReadyChecker struct {
	name   string
	status domainHealth.Status
}

func (m *mockReadyChecker) Name() string { return m.name }
func (m *mockReadyChecker) Check() domainHealth.ComponentStatus {
	return domainHealth.ComponentStatus{
		Name:      m.name,
		Status:    m.status,
		CheckedAt: time.Now(),
	}
}

func TestHealthHandler_LivenessCheck(t *testing.T) {
	svc := health.NewService()
	h := NewHealthHandler(svc)

	r := NewRouter()
	h.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "OK", rec.Body.String())
}

func TestHealthHandler_ReadinessCheck_Ready(t *testing.T) {
	svc := health.NewService(&mockReadyChecker{name: "db", status: domainHealth.StatusHealthy})
	h := NewHealthHandler(svc)

	r := NewRouter()
	h.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/readyz", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "READY", rec.Body.String())
}

func TestHealthHandler_ReadinessCheck_Unready(t *testing.T) {
	svc := health.NewService(&mockReadyChecker{name: "db", status: domainHealth.StatusUnhealthy})
	h := NewHealthHandler(svc)

	r := NewRouter()
	h.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/readyz", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHealthHandler_HealthCheck(t *testing.T) {
	svc := health.NewService(&mockReadyChecker{name: "db", status: domainHealth.StatusHealthy})
	h := NewHealthHandler(svc)

	r := NewRouter()
	h.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp ResponseEnvelope
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotEmpty(t, resp.Meta.RequestID)
}
