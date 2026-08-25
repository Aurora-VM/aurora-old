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
	appIPAM "github.com/aurora-vm/aurora/internal/app/ipam"
	appNetwork "github.com/aurora-vm/aurora/internal/app/network"
	appNode "github.com/aurora-vm/aurora/internal/app/node"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/infra/crypto"
	"github.com/aurora-vm/aurora/internal/infra/jwt"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/aurora-vm/aurora/internal/infra/pki"
	"github.com/aurora-vm/aurora/internal/infra/secrets"
	"github.com/aurora-vm/aurora/internal/infra/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupIPAMHTTPTest(t *testing.T) (*http.Handler, string, string, string, *memory.MemoryStore) {
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

	ipamService := appIPAM.NewService(memStore.IPPools(), memStore.IPAllocations(), authorizer, memStore.Audit())
	computeService := appCompute.NewService(memStore.Instances(), memStore.Nodes(), nodeService, authorizer, memStore.Audit())
	netService := appNetwork.NewService(memStore.Firewall(), memStore.Instances(), memStore.Nodes(), nodeService, authorizer, memStore.Audit())

	authMiddleware := AuthenticateMiddleware(tokenMgr, apiKeyService)
	router := NewRouter()

	ipamHandler := NewIPAMHandler(ipamService, authorizer)
	ipamHandler.RegisterRoutes(router, authMiddleware)

	instHandler := NewInstanceHandler(computeService, netService, authorizer)
	instHandler.RegisterRoutes(router, authMiddleware)

	// Create admin and customer tokens
	adminUser, _ := authService.Register(ctx, auth.RegisterRequest{Username: "admin", Email: "admin@aurora.local", Password: "Password12345!"})
	adminToken, _ := tokenMgr.GenerateAccessToken(adminUser, []string{"superadmin"}, []string{"*"})

	cust1User, _ := authService.Register(ctx, auth.RegisterRequest{Username: "cust1", Email: "cust1@aurora.local", Password: "Password12345!"})
	cust1Token, _ := tokenMgr.GenerateAccessToken(cust1User, []string{"customer"}, []string{"instance:read", "instance:update", "ipam:read"})

	// Create Instance owned by Customer 1
	inst := &domainCompute.Instance{
		ID:        "inst-http-net-01",
		UserID:    cust1User.ID,
		Name:      "prod-db-01",
		Type:      domainCompute.TypeContainer,
		Status:    domainCompute.StatusRunning,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	_ = memStore.Instances().Create(ctx, inst)

	var handler http.Handler = router
	return &handler, adminToken, cust1Token, inst.ID, memStore
}

func TestHTTP_IPAM_And_Firewall_Workflow(t *testing.T) {
	handler, adminToken, cust1Token, instanceID, _ := setupIPAMHTTPTest(t)

	// 1. Customer attempts to create IP pool -> 403 Forbidden
	poolBody, _ := json.Marshal(map[string]interface{}{
		"name":       "Public IPv4 Range",
		"locationId": "us-east-dc1",
		"cidr":       "172.16.50.0/29",
		"gateway":    "172.16.50.1",
	})
	reqCreatePoolCust := httptest.NewRequest("POST", "/api/v1/ipam/pools", bytes.NewReader(poolBody))
	reqCreatePoolCust.Header.Set("Authorization", "Bearer "+cust1Token)
	reqCreatePoolCust.Header.Set("Content-Type", "application/json")
	recCreatePoolCust := httptest.NewRecorder()
	(*handler).ServeHTTP(recCreatePoolCust, reqCreatePoolCust)
	assert.Equal(t, http.StatusForbidden, recCreatePoolCust.Code)

	// 2. Admin creates IP pool -> 201 Created
	reqCreatePoolAdmin := httptest.NewRequest("POST", "/api/v1/ipam/pools", bytes.NewReader(poolBody))
	reqCreatePoolAdmin.Header.Set("Authorization", "Bearer "+adminToken)
	reqCreatePoolAdmin.Header.Set("Content-Type", "application/json")
	recCreatePoolAdmin := httptest.NewRecorder()
	(*handler).ServeHTTP(recCreatePoolAdmin, reqCreatePoolAdmin)
	assert.Equal(t, http.StatusCreated, recCreatePoolAdmin.Code)

	var poolResp ResponseEnvelope
	_ = json.Unmarshal(recCreatePoolAdmin.Body.Bytes(), &poolResp)
	poolData := poolResp.Data.(map[string]interface{})
	poolID := poolData["id"].(string)
	assert.NotEmpty(t, poolID)

	// 3. Customer lists IP pools -> 200 OK
	reqListPools := httptest.NewRequest("GET", "/api/v1/ipam/pools", nil)
	reqListPools.Header.Set("Authorization", "Bearer "+cust1Token)
	recListPools := httptest.NewRecorder()
	(*handler).ServeHTTP(recListPools, reqListPools)
	assert.Equal(t, http.StatusOK, recListPools.Code)

	// 4. Admin allocates IP from pool -> 201 Created
	allocBody, _ := json.Marshal(map[string]interface{}{
		"instanceId":    instanceID,
		"interfaceName": "eth0",
		"notes":         "Primary IPv4",
	})
	reqAlloc := httptest.NewRequest("POST", "/api/v1/ipam/pools/"+poolID+"/allocate", bytes.NewReader(allocBody))
	reqAlloc.Header.Set("Authorization", "Bearer "+adminToken)
	reqAlloc.Header.Set("Content-Type", "application/json")
	recAlloc := httptest.NewRecorder()
	(*handler).ServeHTTP(recAlloc, reqAlloc)
	assert.Equal(t, http.StatusCreated, recAlloc.Code)

	var allocResp ResponseEnvelope
	_ = json.Unmarshal(recAlloc.Body.Bytes(), &allocResp)
	allocData := allocResp.Data.(map[string]interface{})
	allocID := allocData["id"].(string)
	assert.Equal(t, "172.16.50.2", allocData["ipAddress"])

	// 5. Customer applies Firewall Rules on their instance -> 200 OK
	fwBody, _ := json.Marshal(map[string]interface{}{
		"rules": []map[string]interface{}{
			{"direction": "inbound", "action": "allow", "protocol": "tcp", "portRange": "22", "priority": 10},
			{"direction": "inbound", "action": "allow", "protocol": "tcp", "portRange": "80", "priority": 20},
			{"direction": "inbound", "action": "allow", "protocol": "tcp", "portRange": "443", "priority": 30},
			{"direction": "inbound", "action": "drop", "protocol": "all", "portRange": "any", "priority": 100},
		},
	})
	reqFW := httptest.NewRequest("PUT", "/api/v1/instances/"+instanceID+"/firewall", bytes.NewReader(fwBody))
	reqFW.Header.Set("Authorization", "Bearer "+cust1Token)
	reqFW.Header.Set("Content-Type", "application/json")
	recFW := httptest.NewRecorder()
	(*handler).ServeHTTP(recFW, reqFW)
	assert.Equal(t, http.StatusOK, recFW.Code)

	// 6. Customer reads back Firewall Rules -> 200 OK
	reqGetFW := httptest.NewRequest("GET", "/api/v1/instances/"+instanceID+"/firewall", nil)
	reqGetFW.Header.Set("Authorization", "Bearer "+cust1Token)
	recGetFW := httptest.NewRecorder()
	(*handler).ServeHTTP(recGetFW, reqGetFW)
	assert.Equal(t, http.StatusOK, recGetFW.Code)

	var fwResp ResponseEnvelope
	_ = json.Unmarshal(recGetFW.Body.Bytes(), &fwResp)
	rulesList := fwResp.Data.([]interface{})
	assert.Len(t, rulesList, 4)

	// 7. Admin releases allocated IP -> 200 OK
	reqRel := httptest.NewRequest("DELETE", "/api/v1/ipam/allocations/"+allocID, nil)
	reqRel.Header.Set("Authorization", "Bearer "+adminToken)
	recRel := httptest.NewRecorder()
	(*handler).ServeHTTP(recRel, reqRel)
	assert.Equal(t, http.StatusOK, recRel.Code)
}
