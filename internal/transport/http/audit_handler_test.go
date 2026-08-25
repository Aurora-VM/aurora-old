package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aurora-vm/aurora/internal/app/apikeys"
	appAudit "github.com/aurora-vm/aurora/internal/app/audit"
	"github.com/aurora-vm/aurora/internal/app/auth"
	"github.com/aurora-vm/aurora/internal/app/authz"
	domainAudit "github.com/aurora-vm/aurora/internal/domain/audit"
	"github.com/aurora-vm/aurora/internal/infra/crypto"
	"github.com/aurora-vm/aurora/internal/infra/jwt"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/aurora-vm/aurora/internal/infra/secrets"
	"github.com/aurora-vm/aurora/internal/infra/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAuditHTTPTest(t *testing.T) (*http.Handler, string, string, string) {
	ctx := context.Background()
	memStore := memory.NewMemoryStore()
	hasher := crypto.NewArgon2Hasher(nil)
	protector, err := secrets.NewAESGCMProtector("test-master-key-32-characters-long!")
	require.NoError(t, err)
	tokenMgr, err := jwt.NewTokenManager("test-jwt-secret-key-32-characters-long!")
	require.NoError(t, err)
	totpMgr := totp.NewTOTPManager()

	authService := auth.NewService(memStore.Users(), memStore.Roles(), memStore.Sessions(), hasher, protector, tokenMgr, totpMgr, memStore.Audit())
	apiKeyService := apikeys.NewService(memStore.APIKeys(), memStore.Users(), memStore.Roles(), memStore.Audit())
	authorizer := authz.NewAuthorizer(memStore.Roles())

	auditService := appAudit.NewService(memStore.Audit(), memStore.SIEM(), nil, authorizer)

	authMiddleware := AuthenticateMiddleware(tokenMgr, apiKeyService)
	router := NewRouter()

	auditHandler := NewAuditHandler(auditService, authorizer)
	auditHandler.RegisterRoutes(router, authMiddleware)

	// Create admin & customer
	adminUser, _ := authService.Register(ctx, auth.RegisterRequest{Username: "admin", Email: "admin@aurora.local", Password: "Password12345!"})
	adminToken, _ := tokenMgr.GenerateAccessToken(adminUser, []string{"superadmin"}, []string{"*"})

	cust1User, _ := authService.Register(ctx, auth.RegisterRequest{Username: "cust1", Email: "cust1@aurora.local", Password: "Password12345!"})
	cust1Token, _ := tokenMgr.GenerateAccessToken(cust1User, []string{"customer"}, []string{"audit:read"})

	// Record actions
	_ = auditService.Record(ctx, &domainAudit.AuditLog{
		ActorID:      &cust1User.ID,
		Action:       "instance.create",
		ResourceType: "instance",
		Severity:     domainAudit.SeverityInfo,
		CreatedAt:    time.Now().UTC().Add(-1 * time.Minute),
	})

	_ = auditService.Record(ctx, &domainAudit.AuditLog{
		ActorID:      &adminUser.ID,
		Action:       "node.enroll",
		ResourceType: "node",
		Severity:     domainAudit.SeverityInfo,
		CreatedAt:    time.Now().UTC(),
	})

	var handler http.Handler = router
	return &handler, adminToken, cust1Token, cust1User.ID
}

func TestHTTP_Audit_And_SIEM_Workflow(t *testing.T) {
	handler, adminToken, cust1Token, cust1ID := setupAuditHTTPTest(t)

	// 1. Customer lists logs -> only sees own logs (auth.register + instance.create)
	reqCustLogs := httptest.NewRequest("GET", "/api/v1/audit/logs", nil)
	reqCustLogs.Header.Set("Authorization", "Bearer "+cust1Token)
	recCustLogs := httptest.NewRecorder()
	(*handler).ServeHTTP(recCustLogs, reqCustLogs)
	assert.Equal(t, http.StatusOK, recCustLogs.Code)

	var custResp ResponseEnvelope
	_ = json.Unmarshal(recCustLogs.Body.Bytes(), &custResp)
	custData := custResp.Data.(map[string]interface{})
	logsList := custData["logs"].([]interface{})
	assert.Len(t, logsList, 2)
	for _, l := range logsList {
		logObj := l.(map[string]interface{})
		assert.Equal(t, cust1ID, logObj["actorId"])
	}

	// 2. Admin lists logs -> sees all 4 logs
	reqAdminLogs := httptest.NewRequest("GET", "/api/v1/audit/logs", nil)
	reqAdminLogs.Header.Set("Authorization", "Bearer "+adminToken)
	recAdminLogs := httptest.NewRecorder()
	(*handler).ServeHTTP(recAdminLogs, reqAdminLogs)
	assert.Equal(t, http.StatusOK, recAdminLogs.Code)

	// 3. Cryptographic Chain Verification -> 200 OK & valid: true
	reqVerify := httptest.NewRequest("GET", "/api/v1/audit/verify", nil)
	reqVerify.Header.Set("Authorization", "Bearer "+adminToken)
	recVerify := httptest.NewRecorder()
	(*handler).ServeHTTP(recVerify, reqVerify)
	assert.Equal(t, http.StatusOK, recVerify.Code)

	var verifyResp ResponseEnvelope
	_ = json.Unmarshal(recVerify.Body.Bytes(), &verifyResp)
	verifyData := verifyResp.Data.(map[string]interface{})
	assert.Equal(t, true, verifyData["valid"])

	// 4. Export CSV Compliance Report -> 200 OK
	reqCSV := httptest.NewRequest("GET", "/api/v1/audit/export?format=csv", nil)
	reqCSV.Header.Set("Authorization", "Bearer "+adminToken)
	recCSV := httptest.NewRecorder()
	(*handler).ServeHTTP(recCSV, reqCSV)
	assert.Equal(t, http.StatusOK, recCSV.Code)
	assert.Equal(t, "text/csv", recCSV.Header().Get("Content-Type"))
	assert.True(t, strings.Contains(recCSV.Body.String(), "instance.create"))

	// 5. SIEM Destinations (Create -> List -> Delete)
	siemPayload, _ := json.Marshal(map[string]interface{}{
		"name":      "Datadog Webhook",
		"type":      "webhook",
		"target":    "https://http-intake.logs.datadoghq.com/api/v2/logs",
		"authToken": "test-key",
		"format":    "json",
	})
	reqCreateSIEM := httptest.NewRequest("POST", "/api/v1/audit/siem", bytes.NewReader(siemPayload))
	reqCreateSIEM.Header.Set("Authorization", "Bearer "+adminToken)
	reqCreateSIEM.Header.Set("Content-Type", "application/json")
	recCreateSIEM := httptest.NewRecorder()
	(*handler).ServeHTTP(recCreateSIEM, reqCreateSIEM)
	assert.Equal(t, http.StatusCreated, recCreateSIEM.Code)

	var siemResp ResponseEnvelope
	_ = json.Unmarshal(recCreateSIEM.Body.Bytes(), &siemResp)
	siemData := siemResp.Data.(map[string]interface{})
	siemID := siemData["id"].(string)

	reqListSIEM := httptest.NewRequest("GET", "/api/v1/audit/siem", nil)
	reqListSIEM.Header.Set("Authorization", "Bearer "+adminToken)
	recListSIEM := httptest.NewRecorder()
	(*handler).ServeHTTP(recListSIEM, reqListSIEM)
	assert.Equal(t, http.StatusOK, recListSIEM.Code)

	reqDelSIEM := httptest.NewRequest("DELETE", "/api/v1/audit/siem/"+siemID, nil)
	reqDelSIEM.Header.Set("Authorization", "Bearer "+adminToken)
	recDelSIEM := httptest.NewRecorder()
	(*handler).ServeHTTP(recDelSIEM, reqDelSIEM)
	assert.Equal(t, http.StatusOK, recDelSIEM.Code)
}
