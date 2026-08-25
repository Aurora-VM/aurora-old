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
	appNetwork "github.com/aurora-vm/aurora/internal/app/network"
	appNode "github.com/aurora-vm/aurora/internal/app/node"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	"github.com/aurora-vm/aurora/internal/infra/crypto"
	"github.com/aurora-vm/aurora/internal/infra/incus"
	"github.com/aurora-vm/aurora/internal/infra/jwt"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/aurora-vm/aurora/internal/infra/pki"
	"github.com/aurora-vm/aurora/internal/infra/secrets"
	"github.com/aurora-vm/aurora/internal/infra/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockHTTPComputeStreamSender struct {
	driver      domainCompute.HypervisorDriver
	nodeService *appNode.Service
}

func (m *mockHTTPComputeStreamSender) Send(cmd *domainNode.Command) error {
	ctx := context.Background()
	switch cmd.Type {
	case "create_instance":
		spec := &domainCompute.InstanceSpec{
			Name:             cmd.Payload["name"].(string),
			Type:             domainCompute.Type(cmd.Payload["type"].(string)),
			CPUCores:         cmd.Payload["cpu_cores"].(int),
			MemoryBytes:      cmd.Payload["memory_bytes"].(int64),
			StorageBytes:     cmd.Payload["storage_bytes"].(int64),
			Image:            cmd.Payload["image"].(string),
			StartAfterCreate: cmd.Payload["start_after_create"].(bool),
		}
		info, err := m.driver.CreateInstance(ctx, spec)
		success := err == nil
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		payload := map[string]interface{}{}
		if info != nil {
			payload["name"] = info.Name
			payload["status"] = string(info.Status)
			payload["ipv4Address"] = info.IPv4Address
			payload["ipv6Address"] = info.IPv6Address
		}
		go m.nodeService.HandleCommandResult(&domainNode.CommandResult{
			CorrelationID: cmd.CorrelationID,
			Success:       success,
			ErrorMessage:  errMsg,
			Payload:       payload,
			CompletedAt:   time.Now().UTC(),
		})

	case "start_instance":
		name := cmd.Payload["name"].(string)
		err := m.driver.StartInstance(ctx, name)
		go m.nodeService.HandleCommandResult(&domainNode.CommandResult{
			CorrelationID: cmd.CorrelationID,
			Success:       err == nil,
			Payload:       map[string]interface{}{"status": "running"},
			CompletedAt:   time.Now().UTC(),
		})

	case "stop_instance":
		name := cmd.Payload["name"].(string)
		force := cmd.Payload["force"].(bool)
		err := m.driver.StopInstance(ctx, name, force)
		go m.nodeService.HandleCommandResult(&domainNode.CommandResult{
			CorrelationID: cmd.CorrelationID,
			Success:       err == nil,
			Payload:       map[string]interface{}{"status": "stopped"},
			CompletedAt:   time.Now().UTC(),
		})

	case "restart_instance":
		name := cmd.Payload["name"].(string)
		force := cmd.Payload["force"].(bool)
		err := m.driver.RestartInstance(ctx, name, force)
		go m.nodeService.HandleCommandResult(&domainNode.CommandResult{
			CorrelationID: cmd.CorrelationID,
			Success:       err == nil,
			Payload:       map[string]interface{}{"status": "running"},
			CompletedAt:   time.Now().UTC(),
		})

	case "delete_instance":
		name := cmd.Payload["name"].(string)
		force := cmd.Payload["force"].(bool)
		err := m.driver.DeleteInstance(ctx, name, force)
		go m.nodeService.HandleCommandResult(&domainNode.CommandResult{
			CorrelationID: cmd.CorrelationID,
			Success:       err == nil,
			Payload:       map[string]interface{}{"deleted": true},
			CompletedAt:   time.Now().UTC(),
		})

	case "update_instance_spec":
		name := cmd.Payload["name"].(string)
		cpu := cmd.Payload["cpu_cores"].(int)
		mem := cmd.Payload["memory_bytes"].(int64)
		disk := cmd.Payload["storage_bytes"].(int64)
		err := m.driver.UpdateSpec(ctx, name, cpu, mem, disk)
		go m.nodeService.HandleCommandResult(&domainNode.CommandResult{
			CorrelationID: cmd.CorrelationID,
			Success:       err == nil,
			Payload:       map[string]interface{}{"updated": true},
			CompletedAt:   time.Now().UTC(),
		})

	case "get_instance_metrics":
		name := cmd.Payload["name"].(string)
		metrics, err := m.driver.GetMetrics(ctx, name)
		go m.nodeService.HandleCommandResult(&domainNode.CommandResult{
			CorrelationID: cmd.CorrelationID,
			Success:       err == nil,
			Payload: map[string]interface{}{
				"cpuUsagePercent":  metrics.CPUUsagePercent,
				"memoryUsageBytes": metrics.MemoryUsageBytes,
				"memoryPeakBytes":  metrics.MemoryPeakBytes,
			},
			CompletedAt: time.Now().UTC(),
		})
	}
	return nil
}

func setupComputeHTTPTest(t *testing.T) (*http.Handler, *jwt.TokenManager, string, string, string) {
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
	nodeService := appNode.NewService(memStore.Nodes(), memStore.Enrollments(), ca, connMgr, memStore.Audit(), "127.0.0.1:8443")

	// Register online node
	nodeID := "node-http-test-01"
	err = memStore.Nodes().Create(ctx, &domainNode.Node{
		ID:              nodeID,
		Name:            "hv-http-01",
		FQDN:            "127.0.0.1",
		Status:          domainNode.StatusOnline,
		MaintenanceMode: false,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	})
	require.NoError(t, err)

	driver := incus.NewSimulatedDriver()
	mockSender := &mockHTTPComputeStreamSender{driver: driver, nodeService: nodeService}
	err = nodeService.OnStreamConnected(ctx, nodeID, mockSender)
	require.NoError(t, err)

	computeService := appCompute.NewService(memStore.Instances(), memStore.Nodes(), nodeService, authorizer, memStore.Audit())

	netService := appNetwork.NewService(memStore.Firewall(), memStore.Instances(), memStore.Nodes(), nodeService, authorizer, memStore.Audit())

	authMiddleware := AuthenticateMiddleware(tokenMgr, apiKeyService)
	router := NewRouter()

	instHandler := NewInstanceHandler(computeService, netService, authorizer)
	instHandler.RegisterRoutes(router, authMiddleware)

	// Create admin, customer 1, customer 2
	adminUser, _ := authService.Register(ctx, auth.RegisterRequest{Username: "admin", Email: "admin@aurora.local", Password: "Password12345!"})
	adminToken, _ := tokenMgr.GenerateAccessToken(adminUser, []string{"superadmin"}, []string{"*"})

	cust1User, _ := authService.Register(ctx, auth.RegisterRequest{Username: "cust1", Email: "cust1@aurora.local", Password: "Password12345!"})
	cust1Token, _ := tokenMgr.GenerateAccessToken(cust1User, []string{"customer"}, []string{"instance:create", "instance:read", "instance:power", "instance:update", "instance:delete"})

	cust2User, _ := authService.Register(ctx, auth.RegisterRequest{Username: "cust2", Email: "cust2@aurora.local", Password: "Password12345!"})
	cust2Token, _ := tokenMgr.GenerateAccessToken(cust2User, []string{"customer"}, []string{"instance:read", "instance:power"})

	var handler http.Handler = router
	return &handler, tokenMgr, adminToken, cust1Token, cust2Token
}

func TestHTTP_Instance_FullLifecycle_And_Tenancy(t *testing.T) {
	handler, _, adminToken, cust1Token, cust2Token := setupComputeHTTPTest(t)

	// 1. Customer 1 creates an instance
	createBody, _ := json.Marshal(map[string]interface{}{
		"name":             "vps-ubuntu-01",
		"type":             "container",
		"cpuCores":         2,
		"memoryBytes":      2147483648,
		"storageBytes":     21474836480,
		"image":            "images:ubuntu/24.04",
		"startAfterCreate": true,
	})

	reqCreate := httptest.NewRequest("POST", "/api/v1/instances", bytes.NewReader(createBody))
	reqCreate.Header.Set("Authorization", "Bearer "+cust1Token)
	reqCreate.Header.Set("Content-Type", "application/json")
	recCreate := httptest.NewRecorder()
	(*handler).ServeHTTP(recCreate, reqCreate)
	assert.Equal(t, http.StatusCreated, recCreate.Code)

	var createResp ResponseEnvelope
	_ = json.Unmarshal(recCreate.Body.Bytes(), &createResp)
	instData := createResp.Data.(map[string]interface{})
	instanceID := instData["id"].(string)
	assert.NotEmpty(t, instanceID)
	assert.Equal(t, "vps-ubuntu-01", instData["name"])

	// 2. Customer 1 lists instances -> sees 1 instance
	reqList := httptest.NewRequest("GET", "/api/v1/instances", nil)
	reqList.Header.Set("Authorization", "Bearer "+cust1Token)
	recList := httptest.NewRecorder()
	(*handler).ServeHTTP(recList, reqList)
	assert.Equal(t, http.StatusOK, recList.Code)

	var listResp ResponseEnvelope
	_ = json.Unmarshal(recList.Body.Bytes(), &listResp)
	listData := listResp.Data.([]interface{})
	assert.Len(t, listData, 1)

	// 3. Customer 2 lists instances -> sees 0 instances (Tenant Isolation!)
	reqList2 := httptest.NewRequest("GET", "/api/v1/instances", nil)
	reqList2.Header.Set("Authorization", "Bearer "+cust2Token)
	recList2 := httptest.NewRecorder()
	(*handler).ServeHTTP(recList2, reqList2)
	assert.Equal(t, http.StatusOK, recList2.Code)

	var listResp2 ResponseEnvelope
	_ = json.Unmarshal(recList2.Body.Bytes(), &listResp2)
	assert.Empty(t, listResp2.Data)

	// 4. Customer 2 attempts to Stop Customer 1's instance -> 403 Forbidden!
	stopBody, _ := json.Marshal(map[string]interface{}{"action": "stop", "force": false})
	reqStop2 := httptest.NewRequest("POST", "/api/v1/instances/"+instanceID+"/power", bytes.NewReader(stopBody))
	reqStop2.Header.Set("Authorization", "Bearer "+cust2Token)
	reqStop2.Header.Set("Content-Type", "application/json")
	recStop2 := httptest.NewRecorder()
	(*handler).ServeHTTP(recStop2, reqStop2)
	assert.Equal(t, http.StatusForbidden, recStop2.Code)

	// 5. Customer 1 stops their instance -> 200 OK
	reqStop1 := httptest.NewRequest("POST", "/api/v1/instances/"+instanceID+"/power", bytes.NewReader(stopBody))
	reqStop1.Header.Set("Authorization", "Bearer "+cust1Token)
	reqStop1.Header.Set("Content-Type", "application/json")
	recStop1 := httptest.NewRecorder()
	(*handler).ServeHTTP(recStop1, reqStop1)
	assert.Equal(t, http.StatusOK, recStop1.Code)

	// 6. Customer 1 fetches live metrics
	reqMetrics := httptest.NewRequest("GET", "/api/v1/instances/"+instanceID+"/metrics", nil)
	reqMetrics.Header.Set("Authorization", "Bearer "+cust1Token)
	recMetrics := httptest.NewRecorder()
	(*handler).ServeHTTP(recMetrics, reqMetrics)
	assert.Equal(t, http.StatusOK, recMetrics.Code)

	// 7. Customer 1 resizes instance spec
	resizeBody, _ := json.Marshal(map[string]interface{}{
		"cpuCores":     4,
		"memoryBytes":  4294967296,
		"storageBytes": 42949672960,
	})
	reqResize := httptest.NewRequest("PATCH", "/api/v1/instances/"+instanceID+"/spec", bytes.NewReader(resizeBody))
	reqResize.Header.Set("Authorization", "Bearer "+cust1Token)
	reqResize.Header.Set("Content-Type", "application/json")
	recResize := httptest.NewRecorder()
	(*handler).ServeHTTP(recResize, reqResize)
	assert.Equal(t, http.StatusOK, recResize.Code)

	// 8. Superadmin deletes instance -> 200 OK
	reqDel := httptest.NewRequest("DELETE", "/api/v1/instances/"+instanceID, nil)
	reqDel.Header.Set("Authorization", "Bearer "+adminToken)
	recDel := httptest.NewRecorder()
	(*handler).ServeHTTP(recDel, reqDel)
	assert.Equal(t, http.StatusOK, recDel.Code)

	// 9. Query deleted instance -> 404 Not Found
	reqGet := httptest.NewRequest("GET", "/api/v1/instances/"+instanceID, nil)
	reqGet.Header.Set("Authorization", "Bearer "+adminToken)
	recGet := httptest.NewRecorder()
	(*handler).ServeHTTP(recGet, reqGet)
	assert.Equal(t, http.StatusNotFound, recGet.Code)
}
