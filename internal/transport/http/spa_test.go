package http

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterSPARoutes_ServesIndexAndAssets(t *testing.T) {
	// Create temporary dist directory with index.html and asset
	tmpDist := t.TempDir()
	indexPath := filepath.Join(tmpDist, "index.html")
	err := os.WriteFile(indexPath, []byte(`<!DOCTYPE html><html><body><div id="root">Aurora Web</div></body></html>`), 0644)
	require.NoError(t, err)

	assetsDir := filepath.Join(tmpDist, "assets")
	err = os.MkdirAll(assetsDir, 0755)
	require.NoError(t, err)
	cssPath := filepath.Join(assetsDir, "main.css")
	err = os.WriteFile(cssPath, []byte("body { background: #000; }"), 0644)
	require.NoError(t, err)

	router := NewRouter()
	RegisterSPARoutes(router, tmpDist)

	// 1. GET / -> returns 200 index.html
	reqRoot := httptest.NewRequest("GET", "/", nil)
	recRoot := httptest.NewRecorder()
	router.ServeHTTP(recRoot, reqRoot)
	assert.Equal(t, http.StatusOK, recRoot.Code)
	assert.Contains(t, recRoot.Body.String(), "Aurora Web")

	// 2. GET /instances/new -> returns 200 index.html (client-side routing fallback)
	reqRoute := httptest.NewRequest("GET", "/instances/new", nil)
	recRoute := httptest.NewRecorder()
	router.ServeHTTP(recRoute, reqRoute)
	assert.Equal(t, http.StatusOK, recRoute.Code)
	assert.Contains(t, recRoute.Body.String(), "Aurora Web")

	// 3. GET /assets/main.css -> returns 200 CSS content
	reqCSS := httptest.NewRequest("GET", "/assets/main.css", nil)
	recCSS := httptest.NewRecorder()
	router.ServeHTTP(recCSS, reqCSS)
	assert.Equal(t, http.StatusOK, recCSS.Code)
	assert.Contains(t, recCSS.Body.String(), "background: #000")

	// 4. GET /api/v1/unknown -> returns JSON 404 (does NOT serve HTML!)
	reqAPI := httptest.NewRequest("GET", "/api/v1/unknown", nil)
	recAPI := httptest.NewRecorder()
	router.ServeHTTP(recAPI, reqAPI)
	assert.Equal(t, http.StatusNotFound, recAPI.Code)
	assert.Contains(t, recAPI.Body.String(), "NOT_FOUND")
}
