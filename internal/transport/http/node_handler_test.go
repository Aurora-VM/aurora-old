package http

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aurora-vm/aurora/internal/app/apikeys"
	"github.com/aurora-vm/aurora/internal/app/auth"
	"github.com/aurora-vm/aurora/internal/app/authz"
	appNode "github.com/aurora-vm/aurora/internal/app/node"
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

func setupTestNodeHTTPServer(t *testing.T) (*http.Handler, *jwt.TokenManager, *appNode.Service, string, string) {
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

	authMiddleware := AuthenticateMiddleware(tokenMgr, apiKeyService)
	router := NewRouter()

	nodeHandler := NewNodeHandler(nodeService, authorizer)
	nodeHandler.RegisterRoutes(router, authMiddleware)

	// Create admin user and customer user
	adminUser, err := authService.Register(ctx, auth.RegisterRequest{
		Username: "admin",
		Email:    "admin@aurora.local",
		Password: "Password12345!",
	})
	require.NoError(t, err)

	adminToken, err := tokenMgr.GenerateAccessToken(adminUser, []string{"superadmin"}, []string{"*"})
	require.NoError(t, err)

	custUser, err := authService.Register(ctx, auth.RegisterRequest{
		Username: "customer",
		Email:    "cust@aurora.local",
		Password: "Password12345!",
	})
	require.NoError(t, err)

	custToken, err := tokenMgr.GenerateAccessToken(custUser, []string{"customer"}, []string{"instance:read"})
	require.NoError(t, err)

	var handler http.Handler = router
	return &handler, tokenMgr, nodeService, adminToken, custToken
}

func generateNodeTestCSR(t *testing.T, commonName, fqdn string) []byte {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			Organization: []string{"Project Aurora Node"},
			CommonName:   commonName,
		},
		DNSNames: []string{fqdn},
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, privKey)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
}

func TestHTTP_NodeManagement_Flow(t *testing.T) {
	ctx := context.Background()
	handler, _, nodeService, adminToken, custToken := setupTestNodeHTTPServer(t)

	// 1. Customer attempts to create enrollment token -> 403 Forbidden
	tokenBody, _ := json.Marshal(map[string]interface{}{
		"locationId": "loc_1",
		"ttlSeconds": 3600,
	})
	reqCust := httptest.NewRequest("POST", "/api/v1/nodes/enrollment-tokens", bytes.NewReader(tokenBody))
	reqCust.Header.Set("Authorization", "Bearer "+custToken)
	reqCust.Header.Set("Content-Type", "application/json")
	recCust := httptest.NewRecorder()
	(*handler).ServeHTTP(recCust, reqCust)
	assert.Equal(t, http.StatusForbidden, recCust.Code)

	// 2. Admin creates enrollment token -> 201 Created
	reqAdmin := httptest.NewRequest("POST", "/api/v1/nodes/enrollment-tokens", bytes.NewReader(tokenBody))
	reqAdmin.Header.Set("Authorization", "Bearer "+adminToken)
	reqAdmin.Header.Set("Content-Type", "application/json")
	recAdmin := httptest.NewRecorder()
	(*handler).ServeHTTP(recAdmin, reqAdmin)
	assert.Equal(t, http.StatusCreated, recAdmin.Code)

	var tokenResp ResponseEnvelope
	_ = json.Unmarshal(recAdmin.Body.Bytes(), &tokenResp)
	tokenData := tokenResp.Data.(map[string]interface{})
	enrollToken := tokenData["enrollmentToken"].(string)
	assert.NotEmpty(t, enrollToken)

	// 3. Mock node enrollment
	csrPEM := generateNodeTestCSR(t, "hv-rest-01", "127.0.0.1")
	nodeID, _, _, _, err := nodeService.EnrollNode(ctx, enrollToken, "hv-rest-01", "127.0.0.1", csrPEM, map[string]interface{}{"cpu": 8})
	require.NoError(t, err)

	// 4. Admin lists nodes -> 200 OK with 1 node
	reqList := httptest.NewRequest("GET", "/api/v1/nodes", nil)
	reqList.Header.Set("Authorization", "Bearer "+adminToken)
	recList := httptest.NewRecorder()
	(*handler).ServeHTTP(recList, reqList)
	assert.Equal(t, http.StatusOK, recList.Code)

	var listResp ResponseEnvelope
	_ = json.Unmarshal(recList.Body.Bytes(), &listResp)
	nodesList := listResp.Data.([]interface{})
	assert.Len(t, nodesList, 1)

	// 5. Admin toggles maintenance mode
	maintBody, _ := json.Marshal(map[string]bool{"enabled": true})
	reqMaint := httptest.NewRequest("POST", "/api/v1/nodes/"+nodeID+"/maintenance", bytes.NewReader(maintBody))
	reqMaint.Header.Set("Authorization", "Bearer "+adminToken)
	reqMaint.Header.Set("Content-Type", "application/json")
	recMaint := httptest.NewRecorder()
	(*handler).ServeHTTP(recMaint, reqMaint)
	assert.Equal(t, http.StatusOK, recMaint.Code)

	n, _ := nodeService.GetNode(ctx, nodeID)
	assert.Equal(t, domainNode.StatusMaintenance, n.Status)

	// 6. Admin revokes node
	reqRevoke := httptest.NewRequest("POST", "/api/v1/nodes/"+nodeID+"/revoke", nil)
	reqRevoke.Header.Set("Authorization", "Bearer "+adminToken)
	recRevoke := httptest.NewRecorder()
	(*handler).ServeHTTP(recRevoke, reqRevoke)
	assert.Equal(t, http.StatusOK, recRevoke.Code)

	nRevoked, _ := nodeService.GetNode(ctx, nodeID)
	assert.Equal(t, domainNode.StatusRevoked, nRevoked.Status)
}
