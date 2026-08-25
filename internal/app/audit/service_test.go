package audit

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aurora-vm/aurora/internal/app/authz"
	domainAudit "github.com/aurora-vm/aurora/internal/domain/audit"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditService_TamperProofChain_And_Verification(t *testing.T) {
	ctx := context.Background()
	memStore := memory.NewMemoryStore()
	authorizer := authz.NewAuthorizer(memStore.Roles())

	svc := NewService(memStore.Audit(), memStore.SIEM(), nil, authorizer)

	adminSub := &identity.Subject{
		UserID:      "usr_admin",
		Roles:       []string{"superadmin"},
		Permissions: []string{"*"},
	}

	cust1Sub := &identity.Subject{
		UserID:      "usr_cust1",
		Roles:       []string{"customer"},
		Permissions: []string{"audit:read"},
	}

	cust2Sub := &identity.Subject{
		UserID:      "usr_cust2",
		Roles:       []string{"customer"},
		Permissions: []string{"audit:read"},
	}

	// 1. Record series of audit logs
	user1 := "usr_cust1"
	user2 := "usr_cust2"

	_ = svc.Record(ctx, &domainAudit.AuditLog{
		ActorID:      &user1,
		ActorIP:      "198.51.100.10",
		Action:       "instance.create",
		ResourceType: "instance",
		Severity:     domainAudit.SeverityInfo,
		CreatedAt:    time.Now().UTC().Add(-2 * time.Minute),
	})

	_ = svc.Record(ctx, &domainAudit.AuditLog{
		ActorID:      &user2,
		ActorIP:      "198.51.100.20",
		Action:       "volume.create",
		ResourceType: "volume",
		Severity:     domainAudit.SeverityInfo,
		CreatedAt:    time.Now().UTC().Add(-1 * time.Minute),
	})

	_ = svc.Record(ctx, &domainAudit.AuditLog{
		ActorID:      &user1,
		ActorIP:      "198.51.100.10",
		Action:       "instance.start",
		ResourceType: "instance",
		Severity:     domainAudit.SeverityInfo,
		CreatedAt:    time.Now().UTC(),
	})

	// 2. Verify chain integrity
	res, err := svc.VerifyAuditChain(ctx, adminSub, 100)
	require.NoError(t, err)
	assert.True(t, res.Valid)
	assert.Equal(t, int64(3), res.VerifiedCount)

	// 3. Customer 1 only sees their own 2 logs
	cust1Logs, count, err := svc.ListAuditLogs(ctx, cust1Sub, domainAudit.AuditFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
	assert.Len(t, cust1Logs, 2)
	assert.Equal(t, "usr_cust1", *cust1Logs[0].ActorID)

	// 4. Customer 2 only sees their own 1 log
	cust2Logs, count, err := svc.ListAuditLogs(ctx, cust2Sub, domainAudit.AuditFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	assert.Len(t, cust2Logs, 1)
	assert.Equal(t, "usr_cust2", *cust2Logs[0].ActorID)

	// 5. Admin sees all 3 logs
	adminLogs, count, err := svc.ListAuditLogs(ctx, adminSub, domainAudit.AuditFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
	assert.Len(t, adminLogs, 3)

	// 6. Export Compliance Report (CSV and JSON)
	csvData, mimeCSV, err := svc.ExportComplianceReport(ctx, adminSub, domainAudit.AuditFilter{}, "csv")
	require.NoError(t, err)
	assert.Equal(t, "text/csv", mimeCSV)
	assert.True(t, strings.Contains(string(csvData), "instance.create"))

	jsonData, mimeJSON, err := svc.ExportComplianceReport(ctx, adminSub, domainAudit.AuditFilter{}, "json")
	require.NoError(t, err)
	assert.Equal(t, "application/json", mimeJSON)
	assert.True(t, strings.Contains(string(jsonData), "volume.create"))

	// 7. SIEM Destinations CRUD
	dest, err := svc.CreateSIEMDestination(ctx, adminSub, CreateSIEMRequest{
		Name:      "Production Datadog Webhook",
		Type:      domainAudit.SIEMTypeWebhook,
		Target:    "https://http-intake.logs.datadoghq.com/api/v2/logs",
		AuthToken: "dd-api-key-test",
		Format:    domainAudit.SIEMFormatJSON,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, dest.ID)

	dests, err := svc.ListSIEMDestinations(ctx, adminSub)
	require.NoError(t, err)
	assert.Len(t, dests, 1)

	err = svc.DeleteSIEMDestination(ctx, adminSub, dest.ID)
	require.NoError(t, err)

	destsAfter, err := svc.ListSIEMDestinations(ctx, adminSub)
	require.NoError(t, err)
	assert.Len(t, destsAfter, 0)
}
