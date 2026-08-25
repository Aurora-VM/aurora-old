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
	appStorage "github.com/aurora-vm/aurora/internal/app/storage"
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

func setupStorageHTTPTest(t *testing.T) (*http.Handler, string, string, string, string) {
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
	storageService := appStorage.NewService(
		memStore.StoragePools(), memStore.Volumes(), memStore.Snapshots(), memStore.Instances(), memStore.Nodes(), nodeService, authorizer, memStore.Audit(),
	)

	authMiddleware := AuthenticateMiddleware(tokenMgr, apiKeyService)
	router := NewRouter()

	instHandler := NewInstanceHandler(computeService, netService, authorizer)
	instHandler.RegisterRoutes(router, authMiddleware)

	storageHandler := NewStorageHandler(storageService, authorizer)
	storageHandler.RegisterRoutes(router, authMiddleware)

	// Create node
	node := &domainNode.Node{
		ID:        "node-http-storage-01",
		Name:      "hv-storage-01",
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
		"instance:read", "instance:create", "instance:update", "instance:delete",
		"volume:read", "volume:create", "volume:update", "volume:delete",
		"volume:attach", "volume:detach", "volume:snapshot", "volume:restore",
		"storage:read",
	})

	// Create Instance owned by Customer 1
	inst := &domainCompute.Instance{
		ID:        "inst-http-storage-01",
		UserID:    cust1User.ID,
		NodeID:    node.ID,
		Name:      "storage-web-01",
		Type:      domainCompute.TypeContainer,
		Status:    domainCompute.StatusRunning,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	_ = memStore.Instances().Create(ctx, inst)

	var handler http.Handler = router
	return &handler, adminToken, cust1Token, node.ID, inst.ID
}

func TestHTTP_Storage_And_Volume_Workflow(t *testing.T) {
	handler, adminToken, cust1Token, nodeID, instanceID := setupStorageHTTPTest(t)

	// 1. Customer attempts to create Storage Pool -> 403 Forbidden
	poolBody, _ := json.Marshal(map[string]interface{}{
		"nodeId":          nodeID,
		"name":            "btrfs-fast-pool",
		"driver":          "btrfs",
		"totalSpaceBytes": 1099511627776,
	})
	reqCreatePoolCust := httptest.NewRequest("POST", "/api/v1/storage/pools", bytes.NewReader(poolBody))
	reqCreatePoolCust.Header.Set("Authorization", "Bearer "+cust1Token)
	reqCreatePoolCust.Header.Set("Content-Type", "application/json")
	recCreatePoolCust := httptest.NewRecorder()
	(*handler).ServeHTTP(recCreatePoolCust, reqCreatePoolCust)
	assert.Equal(t, http.StatusForbidden, recCreatePoolCust.Code)

	// 2. Admin creates Storage Pool -> 201 Created
	reqCreatePoolAdmin := httptest.NewRequest("POST", "/api/v1/storage/pools", bytes.NewReader(poolBody))
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

	// 3. Customer lists Storage Pools -> 200 OK
	reqListPools := httptest.NewRequest("GET", "/api/v1/storage/pools?nodeId="+nodeID, nil)
	reqListPools.Header.Set("Authorization", "Bearer "+cust1Token)
	recListPools := httptest.NewRecorder()
	(*handler).ServeHTTP(recListPools, reqListPools)
	assert.Equal(t, http.StatusOK, recListPools.Code)

	// 4. Customer creates Volume -> 201 Created
	volBody, _ := json.Marshal(map[string]interface{}{
		"poolId":      poolID,
		"name":        "user-data-vol-1",
		"sizeBytes":   21474836480, // 20 GiB
		"contentType": "filesystem",
		"mountPath":   "/mnt/storage",
	})
	reqCreateVol := httptest.NewRequest("POST", "/api/v1/volumes", bytes.NewReader(volBody))
	reqCreateVol.Header.Set("Authorization", "Bearer "+cust1Token)
	reqCreateVol.Header.Set("Content-Type", "application/json")
	recCreateVol := httptest.NewRecorder()
	(*handler).ServeHTTP(recCreateVol, reqCreateVol)
	assert.Equal(t, http.StatusCreated, recCreateVol.Code)

	var volResp ResponseEnvelope
	_ = json.Unmarshal(recCreateVol.Body.Bytes(), &volResp)
	volData := volResp.Data.(map[string]interface{})
	volID := volData["id"].(string)
	assert.NotEmpty(t, volID)

	// 5. Customer resizes Volume to 40 GiB -> 200 OK
	resizeBody, _ := json.Marshal(map[string]interface{}{
		"sizeBytes": 42949672960,
	})
	reqResize := httptest.NewRequest("PATCH", "/api/v1/volumes/"+volID+"/resize", bytes.NewReader(resizeBody))
	reqResize.Header.Set("Authorization", "Bearer "+cust1Token)
	reqResize.Header.Set("Content-Type", "application/json")
	recResize := httptest.NewRecorder()
	(*handler).ServeHTTP(recResize, reqResize)
	assert.Equal(t, http.StatusOK, recResize.Code)

	// 6. Customer creates Volume Snapshot -> 201 Created
	snapBody, _ := json.Marshal(map[string]interface{}{
		"name": "snap-v1",
	})
	reqSnap := httptest.NewRequest("POST", "/api/v1/volumes/"+volID+"/snapshots", bytes.NewReader(snapBody))
	reqSnap.Header.Set("Authorization", "Bearer "+cust1Token)
	reqSnap.Header.Set("Content-Type", "application/json")
	recSnap := httptest.NewRecorder()
	(*handler).ServeHTTP(recSnap, reqSnap)
	assert.Equal(t, http.StatusCreated, recSnap.Code)

	var snapResp ResponseEnvelope
	_ = json.Unmarshal(recSnap.Body.Bytes(), &snapResp)
	snapData := snapResp.Data.(map[string]interface{})
	snapID := snapData["id"].(string)
	assert.NotEmpty(t, snapID)

	// 7. Customer attaches Volume to instance -> 200 OK
	attachBody, _ := json.Marshal(map[string]interface{}{
		"instanceId": instanceID,
		"mountPath":  "/mnt/custom",
		"readOnly":   false,
	})
	reqAttach := httptest.NewRequest("POST", "/api/v1/volumes/"+volID+"/attach", bytes.NewReader(attachBody))
	reqAttach.Header.Set("Authorization", "Bearer "+cust1Token)
	reqAttach.Header.Set("Content-Type", "application/json")
	recAttach := httptest.NewRecorder()
	(*handler).ServeHTTP(recAttach, reqAttach)
	assert.Equal(t, http.StatusOK, recAttach.Code)

	// 8. Customer restores Snapshot -> 200 OK
	reqRestore := httptest.NewRequest("POST", "/api/v1/volumes/"+volID+"/snapshots/"+snapID+"/restore", nil)
	reqRestore.Header.Set("Authorization", "Bearer "+cust1Token)
	recRestore := httptest.NewRecorder()
	(*handler).ServeHTTP(recRestore, reqRestore)
	assert.Equal(t, http.StatusOK, recRestore.Code)

	// 9. Customer detaches Volume -> 200 OK
	reqDetach := httptest.NewRequest("POST", "/api/v1/volumes/"+volID+"/detach", nil)
	reqDetach.Header.Set("Authorization", "Bearer "+cust1Token)
	recDetach := httptest.NewRecorder()
	(*handler).ServeHTTP(recDetach, reqDetach)
	assert.Equal(t, http.StatusOK, recDetach.Code)

	// 10. Customer deletes Volume -> 200 OK
	reqDel := httptest.NewRequest("DELETE", "/api/v1/volumes/"+volID, nil)
	reqDel.Header.Set("Authorization", "Bearer "+cust1Token)
	recDel := httptest.NewRecorder()
	(*handler).ServeHTTP(recDel, reqDel)
	assert.Equal(t, http.StatusOK, recDel.Code)
}
