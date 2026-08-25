package siem

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/audit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSIEMRepo struct {
	destinations []*audit.SIEMDestination
}

func (m *mockSIEMRepo) Create(ctx context.Context, dest *audit.SIEMDestination) error {
	m.destinations = append(m.destinations, dest)
	return nil
}
func (m *mockSIEMRepo) GetByID(ctx context.Context, id string) (*audit.SIEMDestination, error) {
	for _, d := range m.destinations {
		if d.ID == id {
			return d, nil
		}
	}
	return nil, audit.ErrSIEMDestinationNotFound
}
func (m *mockSIEMRepo) List(ctx context.Context) ([]*audit.SIEMDestination, error) {
	return m.destinations, nil
}
func (m *mockSIEMRepo) Delete(ctx context.Context, id string) error {
	var filtered []*audit.SIEMDestination
	for _, d := range m.destinations {
		if d.ID != id {
			filtered = append(filtered, d)
		}
	}
	m.destinations = filtered
	return nil
}

func TestSIEM_Formatters(t *testing.T) {
	actor := "usr_admin_123"
	log := &audit.AuditLog{
		ID:        42,
		ActorID:   &actor,
		ActorIP:   "192.0.2.1",
		Action:    "instance.create",
		Severity:  audit.SeverityInfo,
		CreatedAt: time.Now().UTC(),
		Details:   map[string]interface{}{"name": "web-01"},
	}

	// 1. JSON
	jsonBytes, err := FormatLog(audit.SIEMFormatJSON, log)
	require.NoError(t, err)
	assert.Contains(t, string(jsonBytes), "instance.create")

	// 2. CEF
	cefBytes, err := FormatLog(audit.SIEMFormatCEF, log)
	require.NoError(t, err)
	assert.Contains(t, string(cefBytes), "CEF:0|Aurora|ControlPlane")
	assert.Contains(t, string(cefBytes), "suser=usr_admin_123")

	// 3. RFC5424
	rfcBytes, err := FormatLog(audit.SIEMFormatRFC5424, log)
	require.NoError(t, err)
	assert.Contains(t, string(rfcBytes), "<134>1")
	assert.Contains(t, string(rfcBytes), "aurora controlplane")
}

func TestSIEM_Dispatcher_WebhookDelivery(t *testing.T) {
	var receivedCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-secret-token", r.Header.Get("Authorization"))
		atomic.AddInt32(&receivedCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	repo := &mockSIEMRepo{
		destinations: []*audit.SIEMDestination{
			{
				ID:        "siem-hook-1",
				Name:      "Splunk Webhook",
				Type:      audit.SIEMTypeWebhook,
				Target:    server.URL,
				AuthToken: "test-secret-token",
				Format:    audit.SIEMFormatJSON,
				Enabled:   true,
			},
		},
	}

	dispatcher := NewDispatcher(repo, 100)
	defer dispatcher.Close()

	actor := "usr_test"
	dispatcher.Dispatch(&audit.AuditLog{
		ID:        101,
		ActorID:   &actor,
		Action:    "auth.login.success",
		Severity:  audit.SeverityInfo,
		CreatedAt: time.Now().UTC(),
	})

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(1), atomic.LoadInt32(&receivedCount))
}
