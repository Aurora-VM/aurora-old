package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aurora-vm/aurora/internal/app/authz"
	"github.com/aurora-vm/aurora/internal/app/node"
	appTmpl "github.com/aurora-vm/aurora/internal/app/template"
	"github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/aurora-vm/aurora/internal/infra/imagesource"
	"github.com/aurora-vm/aurora/internal/infra/jwt"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTemplateHandlerTest(t *testing.T) (http.Handler, *memory.MemoryStore, string, string) {
	memStore := memory.NewMemoryStore()
	authorizer := authz.NewAuthorizer(memStore.Roles())
	tokenManager, err := jwt.NewTokenManager("test-jwt-secret-key-32-characters-long!")
	require.NoError(t, err)
	imgSource := imagesource.NewRegistry([]string{"images", "ubuntu"})
	connMgr := node.NewConnectionManager()
	nodeSvc := node.NewService(memStore.Nodes(), memStore.Enrollments(), nil, connMgr, memStore.Audit(), "127.0.0.1:8443")

	tmplService := appTmpl.NewService(memStore.Templates(), memStore.Images(), memStore.Nodes(), nodeSvc, imgSource, authorizer, memStore.Audit())
	handler := NewTemplateHandler(tmplService, authorizer)

	r := chi.NewRouter()
	authMiddleware := AuthenticateMiddleware(tokenManager, nil)
	handler.RegisterRoutes(r, authMiddleware)

	// Issue Admin Token
	adminUser := &identity.User{ID: "usr-admin-tmpl", Username: "admin"}
	adminToken, err := tokenManager.GenerateAccessToken(adminUser, []string{"superadmin"}, []string{"*"})
	require.NoError(t, err)

	// Issue Customer Token
	custUser := &identity.User{ID: "usr-cust-tmpl", Username: "cust"}
	custToken, err := tokenManager.GenerateAccessToken(custUser, []string{"customer"}, []string{"template:read"})
	require.NoError(t, err)

	return r, memStore, adminToken, custToken
}

func TestTemplateHandler_Customer_List_And_Get(t *testing.T) {
	r, _, _, custToken := setupTemplateHandlerTest(t)

	// 1. List active templates as customer
	req := httptest.NewRequest("GET", "/api/v1/templates", nil)
	req.Header.Set("Authorization", "Bearer "+custToken)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	data := resp["data"].(map[string]interface{})
	templates := data["templates"].([]interface{})
	assert.NotEmpty(t, templates)

	// 2. Get specific template by slug
	req = httptest.NewRequest("GET", "/api/v1/templates/ubuntu-24.04", nil)
	req.Header.Set("Authorization", "Bearer "+custToken)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	tmpl := resp["data"].(map[string]interface{})
	assert.Equal(t, "ubuntu-24.04", tmpl["slug"])
}

func TestTemplateHandler_Admin_CRUD_And_RBAC(t *testing.T) {
	r, _, adminToken, custToken := setupTemplateHandlerTest(t)

	createBody := map[string]interface{}{
		"name":                   "Fedora 40 Cloud",
		"slug":                   "fedora-40",
		"description":            "Fedora 40 Cloud Edition",
		"distribution":           "fedora",
		"version":                "40",
		"release":                "cloud",
		"supportedArchitectures": []string{"x86_64", "aarch64"},
		"supportedInstanceTypes": []string{"container", "virtual-machine"},
		"minDiskBytes":           5 * 1024 * 1024 * 1024,
		"minMemoryBytes":         512 * 1024 * 1024,
		"cloudInitSupported":     true,
	}
	bodyBytes, _ := json.Marshal(createBody)

	// 1. Customer attempts to create template -> 403 Forbidden
	req := httptest.NewRequest("POST", "/api/v1/admin/templates", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+custToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// 2. Admin creates template -> 201 Created
	req = httptest.NewRequest("POST", "/api/v1/admin/templates", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var createdResp map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &createdResp)
	require.NoError(t, err)
	createdTmpl := createdResp["data"].(map[string]interface{})
	tmplID := createdTmpl["id"].(string)
	assert.NotEmpty(t, tmplID)

	// 3. Admin updates template
	patchBody := map[string]interface{}{
		"description": "Updated Fedora 40 Cloud Edition",
	}
	patchBytes, _ := json.Marshal(patchBody)
	req = httptest.NewRequest("PATCH", "/api/v1/admin/templates/"+tmplID, bytes.NewReader(patchBytes))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 4. Admin registers image artifact
	fp := strings.Repeat("c", 64)
	imgBody := map[string]interface{}{
		"templateId":       tmplID,
		"architecture":     "x86_64",
		"instanceType":     string(compute.TypeContainer),
		"incusFingerprint": fp,
		"imageAlias":       "images:fedora/40",
		"sourceRemote":     "images",
		"checksum":         fp,
	}
	imgBytes, _ := json.Marshal(imgBody)
	req = httptest.NewRequest("POST", "/api/v1/admin/images", bytes.NewReader(imgBytes))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var imgResp map[string]interface{}
	err = json.Unmarshal(rec.Body.Bytes(), &imgResp)
	require.NoError(t, err)
	imgData := imgResp["data"].(map[string]interface{})
	imgID := imgData["id"].(string)
	assert.NotEmpty(t, imgID)

	// 5. Admin verifies image artifact
	req = httptest.NewRequest("POST", "/api/v1/admin/images/"+imgID+"/verify", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 6. Admin retires image artifact
	req = httptest.NewRequest("POST", "/api/v1/admin/images/"+imgID+"/retire", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 7. Admin deletes template
	req = httptest.NewRequest("DELETE", "/api/v1/admin/templates/"+tmplID, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}
