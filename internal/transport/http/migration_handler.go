package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	appEvacuation "github.com/aurora-vm/aurora/internal/app/evacuation"
	appMigration "github.com/aurora-vm/aurora/internal/app/migration"
	domainIdentity "github.com/aurora-vm/aurora/internal/domain/identity"
	domainMigration "github.com/aurora-vm/aurora/internal/domain/migration"
	"github.com/go-chi/chi/v5"
)

// MigrationHandler exposes administrative endpoints for workload migrations and evacuations.
type MigrationHandler struct {
	migrationService  *appMigration.Service
	evacuationService *appEvacuation.Service
	authorizer        domainIdentity.Authorizer
}

// NewMigrationHandler constructs the MigrationHandler.
func NewMigrationHandler(
	migrationService *appMigration.Service,
	evacuationService *appEvacuation.Service,
	authorizer domainIdentity.Authorizer,
) *MigrationHandler {
	return &MigrationHandler{
		migrationService:  migrationService,
		evacuationService: evacuationService,
		authorizer:        authorizer,
	}
}

// RegisterRoutes registers migration and evacuation endpoints on the router.
func (h *MigrationHandler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	// Migrations
	r.Route("/api/v1/admin/migrations", func(m chi.Router) {
		m.Use(authMiddleware)
		m.Get("/", h.ListMigrations)
		m.Get("/{id}", h.GetMigration)
	})

	// Workload Migration Trigger
	r.Route("/api/v1/admin/instances", func(inst chi.Router) {
		inst.Use(authMiddleware)
		inst.Post("/{id}/migrate", h.MigrateInstance)
	})

	// Node Evacuation
	r.Route("/api/v1/admin/nodes", func(nodes chi.Router) {
		nodes.Use(authMiddleware)
		nodes.Post("/{id}/drain", h.DrainNode)
		nodes.Post("/{id}/undrain", h.UndrainNode)
		nodes.Post("/{id}/evacuate", h.EvacuateNode)
	})
}

// ListMigrations queries cross-tenant migrations for administrators.
func (h *MigrationHandler) ListMigrations(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin access required")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	filter := domainMigration.MigrationFilter{
		TenantID:     r.URL.Query().Get("tenantId"),
		InstanceID:   r.URL.Query().Get("instanceId"),
		SourceNodeID: r.URL.Query().Get("sourceNodeId"),
		DestNodeID:   r.URL.Query().Get("destNodeId"),
		Status:       domainMigration.Status(r.URL.Query().Get("status")),
		Limit:        limit,
		Offset:       offset,
	}

	migrations, total, err := h.migrationService.ListMigrations(r.Context(), sub, filter)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"migrations": migrations,
		"total":      total,
		"limit":      limit,
		"offset":     offset,
	})
}

// GetMigration returns detail for a specific migration.
func (h *MigrationHandler) GetMigration(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin access required")
		return
	}

	id := chi.URLParam(r, "id")
	mig, err := h.migrationService.GetMigration(r.Context(), sub, id)
	if err != nil {
		RespondError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, mig)
}

// MigrateInstance triggers migration for an instance.
func (h *MigrationHandler) MigrateInstance(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin access required")
		return
	}

	instanceID := chi.URLParam(r, "id")

	var body struct {
		DestNodeID string               `json:"destNodeId"`
		Type       domainMigration.Type `json:"type"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	mig, err := h.migrationService.Migrate(r.Context(), sub, appMigration.MigrateRequest{
		InstanceID: instanceID,
		DestNodeID: body.DestNodeID,
		Type:       body.Type,
	})
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, "MIGRATION_FAILED", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusAccepted, mig)
}

// DrainNode marks a node as draining.
func (h *MigrationHandler) DrainNode(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin access required")
		return
	}

	nodeID := chi.URLParam(r, "id")
	if err := h.evacuationService.DrainNode(r.Context(), sub, nodeID, true); err != nil {
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"message": "Node drain mode enabled",
	})
}

// UndrainNode restores a node from draining mode.
func (h *MigrationHandler) UndrainNode(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin access required")
		return
	}

	nodeID := chi.URLParam(r, "id")
	if err := h.evacuationService.DrainNode(r.Context(), sub, nodeID, false); err != nil {
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"message": "Node drain mode disabled (online)",
	})
}

// EvacuateNode drains a node and migrates all hosted instances to healthy target nodes.
func (h *MigrationHandler) EvacuateNode(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin access required")
		return
	}

	nodeID := chi.URLParam(r, "id")
	var body struct {
		DestNodeID string `json:"destNodeId"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	result, err := h.evacuationService.EvacuateNode(r.Context(), sub, nodeID, body.DestNodeID)
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, "EVACUATION_FAILED", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusAccepted, result)
}
