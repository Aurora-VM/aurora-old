package monitoring

import (
	"context"
	"testing"
	"time"

	"github.com/aurora-vm/aurora/internal/app/authz"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainMonitoring "github.com/aurora-vm/aurora/internal/domain/monitoring"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMonitoringService_Ingestion_Query_And_Alerts(t *testing.T) {
	ctx := context.Background()
	memStore := memory.NewMemoryStore()
	authorizer := authz.NewAuthorizer(memStore.Roles())

	svc := NewService(
		memStore.Metrics(),
		memStore.AlertThresholds(),
		memStore.AlertEvents(),
		memStore.Instances(),
		memStore.Nodes(),
		authorizer,
		memStore.Audit(),
	)

	adminSubject := &identity.Subject{
		UserID:      "usr_admin",
		Roles:       []string{"superadmin"},
		Permissions: []string{"*"},
	}

	cust1Subject := &identity.Subject{
		UserID: "usr_cust_1",
		Roles:  []string{"customer"},
		Permissions: []string{
			"instance:read", "monitoring:read", "monitoring:manage",
		},
	}

	cust2Subject := &identity.Subject{
		UserID: "usr_cust_2",
		Roles:  []string{"customer"},
		Permissions: []string{
			"instance:read", "monitoring:read", "monitoring:manage",
		},
	}

	// 1. Create Node & Instance
	node := &domainNode.Node{
		ID:        "node-mon-01",
		Name:      "hv-mon-01",
		FQDN:      "127.0.0.1",
		Status:    domainNode.StatusOnline,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	err := memStore.Nodes().Create(ctx, node)
	require.NoError(t, err)

	inst := &domainCompute.Instance{
		ID:        "inst-mon-01",
		UserID:    cust1Subject.UserID,
		NodeID:    node.ID,
		Name:      "mon-web-01",
		Type:      domainCompute.TypeContainer,
		Status:    domainCompute.StatusRunning,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	err = memStore.Instances().Create(ctx, inst)
	require.NoError(t, err)

	// 2. Create Alert Threshold for high CPU (> 80%) on Instance
	thresh, err := svc.CreateThreshold(ctx, cust1Subject, CreateThresholdRequest{
		ResourceType:    domainMonitoring.ResourceTypeInstance,
		ResourceID:      inst.ID,
		MetricName:      "cpu_percent",
		Operator:        domainMonitoring.OpGT,
		ThresholdValue:  80.0,
		DurationSeconds: 30,
		Severity:        domainMonitoring.SeverityCritical,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, thresh.ID)

	// 3. Ingest Metric below threshold (50%) -> No alert
	now := time.Now().UTC()
	err = svc.IngestMetrics(ctx, []*domainMonitoring.MetricSample{
		{ResourceType: domainMonitoring.ResourceTypeInstance, ResourceID: inst.ID, MetricName: "cpu_percent", Value: 50.0, Timestamp: now.Add(-20 * time.Second)},
		{ResourceType: domainMonitoring.ResourceTypeNode, ResourceID: node.ID, MetricName: "cpu_percent", Value: 25.0, Timestamp: now.Add(-20 * time.Second)},
	})
	require.NoError(t, err)

	alerts, err := svc.ListAlertEvents(ctx, cust1Subject, domainMonitoring.ResourceTypeInstance, inst.ID, domainMonitoring.AlertStateFiring)
	require.NoError(t, err)
	assert.Len(t, alerts, 0)

	// 4. Ingest Metric above threshold (92.5%) -> Alert fires!
	err = svc.IngestMetrics(ctx, []*domainMonitoring.MetricSample{
		{ResourceType: domainMonitoring.ResourceTypeInstance, ResourceID: inst.ID, MetricName: "cpu_percent", Value: 92.5, Timestamp: now},
	})
	require.NoError(t, err)

	alerts, err = svc.ListAlertEvents(ctx, cust1Subject, domainMonitoring.ResourceTypeInstance, inst.ID, domainMonitoring.AlertStateFiring)
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	assert.Equal(t, domainMonitoring.AlertStateFiring, alerts[0].State)
	assert.Equal(t, 92.5, alerts[0].TriggeredValue)
	assert.Equal(t, domainMonitoring.SeverityCritical, alerts[0].Severity)

	// 5. Acknowledge and Resolve Alert
	ackEvent, err := svc.AcknowledgeAlert(ctx, cust1Subject, alerts[0].ID)
	require.NoError(t, err)
	assert.Equal(t, domainMonitoring.AlertStateAcknowledged, ackEvent.State)

	resolvedEvent, err := svc.ResolveAlert(ctx, cust1Subject, alerts[0].ID)
	require.NoError(t, err)
	assert.Equal(t, domainMonitoring.AlertStateResolved, resolvedEvent.State)
	assert.NotNil(t, resolvedEvent.ResolvedAt)

	// 6. Query Instance Metrics
	seriesMap, err := svc.QueryInstanceMetrics(ctx, cust1Subject, inst.ID, []string{"cpu_percent"}, now.Add(-1*time.Minute), now.Add(1*time.Minute), 10)
	require.NoError(t, err)
	assert.Contains(t, seriesMap, "cpu_percent")
	assert.NotEmpty(t, seriesMap["cpu_percent"].DataPoints)

	// 7. Customer 2 cannot query Customer 1's instance metrics -> 403 Forbidden
	_, err = svc.QueryInstanceMetrics(ctx, cust2Subject, inst.ID, []string{"cpu_percent"}, now.Add(-1*time.Minute), now.Add(1*time.Minute), 10)
	assert.Error(t, err)

	// 8. Admin queries Node Metrics
	nodeSeriesMap, err := svc.QueryNodeMetrics(ctx, adminSubject, node.ID, []string{"cpu_percent"}, now.Add(-1*time.Minute), now.Add(1*time.Minute), 10)
	require.NoError(t, err)
	assert.Contains(t, nodeSeriesMap, "cpu_percent")
}
