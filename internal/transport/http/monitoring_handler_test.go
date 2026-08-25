package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aurora-vm/aurora/internal/app/apikeys"
	"github.com/aurora-vm/aurora/internal/app/auth"
	"github.com/aurora-vm/aurora/internal/app/authz"
	appCompute "github.com/aurora-vm/aurora/internal/app/compute"
	appMonitoring "github.com/aurora-vm/aurora/internal/app/monitoring"
	appNetwork "github.com/aurora-vm/aurora/internal/app/network"
	appNode "github.com/aurora-vm/aurora/internal/app/node"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	"github.com/aurora-vm/aurora/internal/infra/crypto"
	"github.com/aurora-vm/aurora/internal/infra/jwt"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/aurora-vm/aurora/internal/infra/pki"
	"github.com/aurora-vm/aurora/internal/infra/secrets"
	"github.com/aurora-vm/aurora/internal/infra/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupMonitoringHTTPTest(t *testing.T) (*http.Handler, string, string, string, string, *memory.MemoryStore) {
	ctx := context.Background()
	memStore := memory.NewMemoryStore()
	hasher := crypto.NewArgon2Hasher(nil)
	protector, err := secrets.NewAESGCMProtector("test-master-key-32-characters-long!")
	require.NoError(t, err)
	tokenMgr, err := jwt.NewTokenManager("test-jwt-secret-key-32-characters-long!")
	require.NoError(t, err)
	totpMgr := totp.NewTOTPManager()
	ca, err := pki.NewInternalCA(nil, nil)
	require.NoError(t, err)

	authService := auth.NewService(memStore.Users(), memStore.Roles(), memStore.Sessions(), hasher, protector, tokenMgr, totpMgr, memStore.Audit())
	apiKeyService := apikeys.NewService(memStore.APIKeys(), memStore.Users(), memStore.Roles(), memStore.Audit())
	authorizer := authz.NewAuthorizer(memStore.Roles())

	connMgr := appNode.NewConnectionManager()
	nodeService := appNode.NewService(memStore.Nodes(), memStore.Enrollments(), ca, connMgr, memStore.Audit(), "127.0.0.1:9443")
	computeService := appCompute.NewService(memStore.Instances(), memStore.Nodes(), nodeService, authorizer, memStore.Audit())
	netService := appNetwork.NewService(memStore.Firewall(), memStore.Instances(), memStore.Nodes(), nodeService, authorizer, memStore.Audit())
	monService := appMonitoring.NewService(
		memStore.Metrics(), memStore.AlertThresholds(), memStore.AlertEvents(), memStore.Instances(), memStore.Nodes(), authorizer, memStore.Audit(),
	)

	authMiddleware := AuthenticateMiddleware(tokenMgr, apiKeyService)
	router := NewRouter()

	instHandler := NewInstanceHandler(computeService, netService, authorizer)
	instHandler.RegisterRoutes(router, authMiddleware)

	monHandler := NewMonitoringHandler(monService, authorizer)
	monHandler.RegisterRoutes(router, authMiddleware)

	// Create node
	node := &domainNode.Node{
		ID:        "node-http-mon-01",
		Name:      "hv-mon-01",
		FQDN:      "127.0.0.1",
		Status:    domainNode.StatusOnline,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	_ = memStore.Nodes().Create(ctx, node)

	// Create admin & customer
	adminUser, _ := authService.Register(ctx, auth.RegisterRequest{Username: "admin", Email: "admin@aurora.local", Password: "Password12345!"})
	adminToken, _ := tokenMgr.GenerateAccessToken(adminUser, []string{"superadmin"}, []string{"*"})

	cust1User, _ := authService.Register(ctx, auth.RegisterRequest{Username: "cust1", Email: "cust1@aurora.local", Password: "Password12345!"})
	cust1Token, _ := tokenMgr.GenerateAccessToken(cust1User, []string{"customer"}, []string{
		"instance:read", "instance:create", "instance:update", "monitoring:read", "monitoring:manage",
	})

	// Create Instance owned by Customer 1
	inst := &domainCompute.Instance{
		ID:        "inst-http-mon-01",
		UserID:    cust1User.ID,
		NodeID:    node.ID,
		Name:      "mon-api-01",
		Type:      domainCompute.TypeContainer,
		Status:    domainCompute.StatusRunning,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	_ = memStore.Instances().Create(ctx, inst)

	var handler http.Handler = router
	return &handler, adminToken, cust1Token, node.ID, inst.ID, memStore
}

func TestHTTP_Monitoring_Workflow(t *testing.T) {
	handler, adminToken, cust1Token, nodeID, instanceID, _ := setupMonitoringHTTPTest(t)

	now := time.Now().UTC()

	// 1. Ingest Telemetry Metrics (Admin / Node) -> 200 OK
	metricsPayload, _ := json.Marshal(map[string]interface{}{
		"samples": []map[string]interface{}{
			{
				"resourceType": "instance",
				"resourceId":   instanceID,
				"metricName":   "cpu_percent",
				"value":        88.5,
				"timestamp":    now.Format(time.RFC3339),
			},
			{
				"resourceType": "node",
				"resourceId":   nodeID,
				"metricName":   "cpu_percent",
				"value":        45.0,
				"timestamp":    now.Format(time.RFC3339),
			},
		},
	})
	reqIngest := httptest.NewRequest("POST", "/api/v1/monitoring/metrics", bytes.NewReader(metricsPayload))
	reqIngest.Header.Set("Authorization", "Bearer "+adminToken)
	reqIngest.Header.Set("Content-Type", "application/json")
	recIngest := httptest.NewRecorder()
	(*handler).ServeHTTP(recIngest, reqIngest)
	assert.Equal(t, http.StatusOK, recIngest.Code)

	// 2. Customer queries instance metrics -> 200 OK
	reqInstMetrics := httptest.NewRequest("GET", "/api/v1/monitoring/instances/"+instanceID+"/metrics?metrics=cpu_percent", nil)
	reqInstMetrics.Header.Set("Authorization", "Bearer "+cust1Token)
	recInstMetrics := httptest.NewRecorder()
	(*handler).ServeHTTP(recInstMetrics, reqInstMetrics)
	assert.Equal(t, http.StatusOK, recInstMetrics.Code)

	var instMetricsResp ResponseEnvelope
	_ = json.Unmarshal(recInstMetrics.Body.Bytes(), &instMetricsResp)
	seriesMap := instMetricsResp.Data.(map[string]interface{})
	assert.Contains(t, seriesMap, "cpu_percent")

	// 3. Customer creates Alert Threshold (cpu_percent > 75%) -> 201 Created
	threshPayload, _ := json.Marshal(map[string]interface{}{
		"resourceType":    "instance",
		"resourceId":      instanceID,
		"metricName":      "cpu_percent",
		"operator":        "gt",
		"thresholdValue":  75.0,
		"durationSeconds": 30,
		"severity":        "critical",
	})
	reqCreateThresh := httptest.NewRequest("POST", "/api/v1/monitoring/thresholds", bytes.NewReader(threshPayload))
	reqCreateThresh.Header.Set("Authorization", "Bearer "+cust1Token)
	reqCreateThresh.Header.Set("Content-Type", "application/json")
	recCreateThresh := httptest.NewRecorder()
	(*handler).ServeHTTP(recCreateThresh, reqCreateThresh)
	assert.Equal(t, http.StatusCreated, recCreateThresh.Code)

	// 4. Ingest metric exceeding threshold (95.0%) -> triggers alert!
	metricsHighCPU, _ := json.Marshal(map[string]interface{}{
		"samples": []map[string]interface{}{
			{
				"resourceType": "instance",
				"resourceId":   instanceID,
				"metricName":   "cpu_percent",
				"value":        95.0,
				"timestamp":    now.Add(10 * time.Second).Format(time.RFC3339),
			},
		},
	})
	reqIngestHigh := httptest.NewRequest("POST", "/api/v1/monitoring/metrics", bytes.NewReader(metricsHighCPU))
	reqIngestHigh.Header.Set("Authorization", "Bearer "+adminToken)
	reqIngestHigh.Header.Set("Content-Type", "application/json")
	recIngestHigh := httptest.NewRecorder()
	(*handler).ServeHTTP(recIngestHigh, reqIngestHigh)
	assert.Equal(t, http.StatusOK, recIngestHigh.Code)

	// 5. Customer queries active alerts -> 200 OK
	reqListAlerts := httptest.NewRequest("GET", "/api/v1/monitoring/alerts?resourceType=instance&resourceId="+instanceID, nil)
	reqListAlerts.Header.Set("Authorization", "Bearer "+cust1Token)
	recListAlerts := httptest.NewRecorder()
	(*handler).ServeHTTP(recListAlerts, reqListAlerts)
	assert.Equal(t, http.StatusOK, recListAlerts.Code)

	var alertsResp ResponseEnvelope
	_ = json.Unmarshal(recListAlerts.Body.Bytes(), &alertsResp)
	alertsList := alertsResp.Data.([]interface{})
	require.NotEmpty(t, alertsList)
	alertObj := alertsList[0].(map[string]interface{})
	alertID := alertObj["id"].(string)
	assert.Equal(t, "firing", alertObj["state"])

	// 6. Acknowledge alert -> 200 OK
	reqAck := httptest.NewRequest("POST", "/api/v1/monitoring/alerts/"+alertID+"/ack", nil)
	reqAck.Header.Set("Authorization", "Bearer "+cust1Token)
	recAck := httptest.NewRecorder()
	(*handler).ServeHTTP(recAck, reqAck)
	assert.Equal(t, http.StatusOK, recAck.Code)

	// 7. Resolve alert -> 200 OK
	reqResolve := httptest.NewRequest("POST", "/api/v1/monitoring/alerts/"+alertID+"/resolve", nil)
	reqResolve.Header.Set("Authorization", "Bearer "+cust1Token)
	recResolve := httptest.NewRecorder()
	(*handler).ServeHTTP(recResolve, reqResolve)
	assert.Equal(t, http.StatusOK, recResolve.Code)

	var resolveResp ResponseEnvelope
	_ = json.Unmarshal(recResolve.Body.Bytes(), &resolveResp)
	resolvedObj := resolveResp.Data.(map[string]interface{})
	assert.Equal(t, "resolved", resolvedObj["state"])
}
