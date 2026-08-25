package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aurora-vm/aurora/internal/app/account"
	"github.com/aurora-vm/aurora/internal/app/apikeys"
	"github.com/aurora-vm/aurora/internal/app/auth"
	"github.com/aurora-vm/aurora/internal/app/authz"
	"github.com/aurora-vm/aurora/internal/infra/crypto"
	"github.com/aurora-vm/aurora/internal/infra/jwt"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/aurora-vm/aurora/internal/infra/secrets"
	"github.com/aurora-vm/aurora/internal/infra/totp"
	"github.com/go-chi/chi/v5"
	pquernaTotp "github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestHTTPServer(t *testing.T) (*chi.Mux, *jwt.TokenManager, *apikeys.Service) {
	memStore := memory.NewMemoryStore()
	hasher := crypto.NewArgon2Hasher(nil)
	protector, err := secrets.NewAESGCMProtector("test-master-key-32-characters-long!")
	require.NoError(t, err)
	tokenMgr, err := jwt.NewTokenManager("test-jwt-secret-key-32-characters-long!")
	require.NoError(t, err)
	totpMgr := totp.NewTOTPManager()

	authService := auth.NewService(memStore.Users(), memStore.Roles(), memStore.Sessions(), hasher, protector, tokenMgr, totpMgr, memStore.Audit())
	acctService := account.NewService(memStore.Users(), hasher, protector, totpMgr, memStore.Audit())
	apiKeyService := apikeys.NewService(memStore.APIKeys(), memStore.Users(), memStore.Roles(), memStore.Audit())
	authorizer := authz.NewAuthorizer(memStore.Roles())

	authMiddleware := AuthenticateMiddleware(tokenMgr, apiKeyService)

	router := NewRouter()

	authHandler := NewAuthHandler(authService)
	authHandler.RegisterRoutes(router, authMiddleware)

	accountHandler := NewAccountHandler(acctService)
	accountHandler.RegisterRoutes(router, authMiddleware)

	apiKeyHandler := NewAPIKeyHandler(apiKeyService, authorizer)
	apiKeyHandler.RegisterRoutes(router, authMiddleware)

	// Protected test endpoint with permission enforcement
	router.Group(func(r chi.Router) {
		r.Use(authMiddleware)
		r.Use(RequirePermission(authorizer, "node:maintenance"))
		r.Get("/api/v1/test/node-maintenance", func(w http.ResponseWriter, r *http.Request) {
			RespondJSON(w, r, http.StatusOK, map[string]string{"status": "maintenance_granted"})
		})
	})

	return router, tokenMgr, apiKeyService
}

func TestHTTP_AuthWorkflow_FullCycle(t *testing.T) {
	r, _, _ := setupTestHTTPServer(t)

	// 1. Register superadmin
	regBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"email":    "admin@aurora.local",
		"password": "Password12345!",
	})
	req := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(regBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)

	// 2. Register customer
	regBody2, _ := json.Marshal(map[string]string{
		"username": "customer",
		"email":    "customer@example.com",
		"password": "Password12345!",
	})
	req2 := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(regBody2))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusCreated, rec2.Code)

	// 3. Login as superadmin
	loginBody, _ := json.Marshal(map[string]string{
		"usernameOrEmail": "admin",
		"password":        "Password12345!",
	})
	reqLogin := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(loginBody))
	reqLogin.Header.Set("Content-Type", "application/json")
	recLogin := httptest.NewRecorder()
	r.ServeHTTP(recLogin, reqLogin)
	assert.Equal(t, http.StatusOK, recLogin.Code)

	var loginResp ResponseEnvelope
	err := json.Unmarshal(recLogin.Body.Bytes(), &loginResp)
	require.NoError(t, err)
	loginData := loginResp.Data.(map[string]interface{})
	tokenData := loginData["tokens"].(map[string]interface{})
	adminAccessToken := tokenData["accessToken"].(string)
	adminRefreshToken := tokenData["refreshToken"].(string)

	// 4. Access /api/v1/auth/me with Bearer token
	reqMe := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	reqMe.Header.Set("Authorization", "Bearer "+adminAccessToken)
	recMe := httptest.NewRecorder()
	r.ServeHTTP(recMe, reqMe)
	assert.Equal(t, http.StatusOK, recMe.Code)

	// 5. Test Superadmin RBAC check on node:maintenance -> Allowed (200)
	reqMaint := httptest.NewRequest("GET", "/api/v1/test/node-maintenance", nil)
	reqMaint.Header.Set("Authorization", "Bearer "+adminAccessToken)
	recMaint := httptest.NewRecorder()
	r.ServeHTTP(recMaint, reqMaint)
	assert.Equal(t, http.StatusOK, recMaint.Code)

	// 6. Login as customer
	loginCustBody, _ := json.Marshal(map[string]string{
		"usernameOrEmail": "customer",
		"password":        "Password12345!",
	})
	reqCustLogin := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(loginCustBody))
	recCustLogin := httptest.NewRecorder()
	r.ServeHTTP(recCustLogin, reqCustLogin)
	var custResp ResponseEnvelope
	_ = json.Unmarshal(recCustLogin.Body.Bytes(), &custResp)
	custData := custResp.Data.(map[string]interface{})
	custTokenData := custData["tokens"].(map[string]interface{})
	custAccessToken := custTokenData["accessToken"].(string)

	// 7. Test Customer RBAC check on node:maintenance -> Denied (403 Forbidden)
	reqCustMaint := httptest.NewRequest("GET", "/api/v1/test/node-maintenance", nil)
	reqCustMaint.Header.Set("Authorization", "Bearer "+custAccessToken)
	recCustMaint := httptest.NewRecorder()
	r.ServeHTTP(recCustMaint, reqCustMaint)
	assert.Equal(t, http.StatusForbidden, recCustMaint.Code)

	// 8. Refresh Token Rotation
	refBody, _ := json.Marshal(map[string]string{"refreshToken": adminRefreshToken})
	reqRef := httptest.NewRequest("POST", "/api/v1/auth/refresh", bytes.NewReader(refBody))
	reqRef.Header.Set("Content-Type", "application/json")
	recRef := httptest.NewRecorder()
	r.ServeHTTP(recRef, reqRef)
	assert.Equal(t, http.StatusOK, recRef.Code)
}

func TestHTTP_2FA_And_APIKeys(t *testing.T) {
	r, _, _ := setupTestHTTPServer(t)

	// 1. Register & Login
	regBody, _ := json.Marshal(map[string]string{
		"username": "alice",
		"email":    "alice@aurora.local",
		"password": "Password12345!",
	})
	req := httptest.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(regBody))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	loginBody, _ := json.Marshal(map[string]string{"usernameOrEmail": "alice", "password": "Password12345!"})
	reqLogin := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(loginBody))
	recLogin := httptest.NewRecorder()
	r.ServeHTTP(recLogin, reqLogin)

	var loginResp ResponseEnvelope
	_ = json.Unmarshal(recLogin.Body.Bytes(), &loginResp)
	tokenData := loginResp.Data.(map[string]interface{})["tokens"].(map[string]interface{})
	token := tokenData["accessToken"].(string)

	// 2. Setup 2FA
	reqSetup := httptest.NewRequest("POST", "/api/v1/account/2fa/setup", nil)
	reqSetup.Header.Set("Authorization", "Bearer "+token)
	recSetup := httptest.NewRecorder()
	r.ServeHTTP(recSetup, reqSetup)
	assert.Equal(t, http.StatusOK, recSetup.Code)

	var setupResp ResponseEnvelope
	_ = json.Unmarshal(recSetup.Body.Bytes(), &setupResp)
	totpSecret := setupResp.Data.(map[string]interface{})["secret"].(string)

	// 3. Enable 2FA
	validCode, _ := pquernaTotp.GenerateCode(totpSecret, time.Now())
	enableBody, _ := json.Marshal(map[string]string{"secret": totpSecret, "code": validCode})
	reqEnable := httptest.NewRequest("POST", "/api/v1/account/2fa/enable", bytes.NewReader(enableBody))
	reqEnable.Header.Set("Authorization", "Bearer "+token)
	recEnable := httptest.NewRecorder()
	r.ServeHTTP(recEnable, reqEnable)
	assert.Equal(t, http.StatusOK, recEnable.Code)

	// 4. Create API Key
	keyBody, _ := json.Marshal(map[string]interface{}{
		"name":   "CLI Token",
		"scopes": []string{"instance:read"},
	})
	reqKey := httptest.NewRequest("POST", "/api/v1/api-keys", bytes.NewReader(keyBody))
	reqKey.Header.Set("Authorization", "Bearer "+token)
	recKey := httptest.NewRecorder()
	r.ServeHTTP(recKey, reqKey)
	assert.Equal(t, http.StatusCreated, recKey.Code)

	var keyResp ResponseEnvelope
	_ = json.Unmarshal(recKey.Body.Bytes(), &keyResp)
	keyData := keyResp.Data.(map[string]interface{})
	plaintextAPIKey := keyData["plaintextKey"].(string)

	// 5. Query API with X-API-Key header
	reqAPI := httptest.NewRequest("GET", "/api/v1/api-keys", nil)
	reqAPI.Header.Set("X-API-Key", plaintextAPIKey)
	recAPI := httptest.NewRecorder()
	r.ServeHTTP(recAPI, reqAPI)
	assert.Equal(t, http.StatusOK, recAPI.Code)
}
