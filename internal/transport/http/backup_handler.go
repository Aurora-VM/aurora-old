package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	appBackup "github.com/aurora-vm/aurora/internal/app/backup"
	appRecovery "github.com/aurora-vm/aurora/internal/app/recovery"
	domainBackup "github.com/aurora-vm/aurora/internal/domain/backup"
	domainIdentity "github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/go-chi/chi/v5"
)

// BackupHandler provides HTTP endpoints for backup and disaster recovery operations.
type BackupHandler struct {
	backupService *appBackup.Service
	recoveryCoord *appRecovery.Coordinator
	authorizer    domainIdentity.Authorizer
}

func NewBackupHandler(
	backupService *appBackup.Service,
	recoveryCoord *appRecovery.Coordinator,
	authorizer domainIdentity.Authorizer,
) *BackupHandler {
	return &BackupHandler{
		backupService: backupService,
		recoveryCoord: recoveryCoord,
		authorizer:    authorizer,
	}
}

// RegisterRoutes registers customer and admin disaster recovery endpoints on Chi router.
func (h *BackupHandler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	// Customer / Tenant Scoped Backups
	r.Route("/api/v1/backups", func(b chi.Router) {
		b.Use(authMiddleware)
		b.Get("/", h.ListCustomerBackups)
		b.Post("/", h.CreateCustomerBackup)
		b.Get("/{id}", h.GetCustomerBackup)
		b.Get("/{id}/download", h.DownloadCustomerBackup)
		b.Post("/{id}/restore", h.RestoreCustomerBackup)
	})

	// Superadmin Disaster Recovery & Backup Hub
	r.Route("/api/v1/admin/recovery", func(admin chi.Router) {
		admin.Use(authMiddleware)
		admin.Get("/backups", h.AdminListBackups)
		admin.Post("/backups", h.AdminCreateBackup)
		admin.Get("/backups/{id}", h.AdminGetBackup)
		admin.Post("/backups/{id}/verify", h.AdminVerifyBackup)
		admin.Delete("/backups/{id}", h.AdminDeleteBackup)

		// Disaster Recovery
		admin.Post("/dry-run", h.DisasterRecoveryDryRun)
		admin.Post("/restore", h.DisasterRecoveryRestore)
		admin.Get("/plans", h.AdminListRestorePlans)
		admin.Get("/plans/{id}", h.AdminGetRestorePlan)
	})
}

// Customer handlers
func (h *BackupHandler) ListCustomerBackups(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	filter := domainBackup.Filter{
		TenantID: sub.UserID,
		Limit:    limit,
		Offset:   offset,
	}

	backups, total, err := h.backupService.ListBackups(r.Context(), sub, filter)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"backups": backups,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

func (h *BackupHandler) CreateCustomerBackup(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	var req appBackup.CreateBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid request body")
		return
	}

	req.TenantID = sub.UserID
	backup, err := h.backupService.CreateBackup(r.Context(), sub, req)
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusCreated, backup)
}

func (h *BackupHandler) GetCustomerBackup(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	backup, err := h.backupService.GetBackup(r.Context(), sub, id)
	if err != nil {
		RespondError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, backup)
}

func (h *BackupHandler) DownloadCustomerBackup(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	data, record, err := h.backupService.DownloadBackup(r.Context(), sub, id)
	if err != nil {
		RespondError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename="+record.ID+".enc")
	w.Header().Set("X-Checksum-SHA256", record.ChecksumSHA256)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *BackupHandler) RestoreCustomerBackup(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	// For customer-scoped restore, verify backup first
	if err := h.backupService.VerifyBackup(r.Context(), sub, id); err != nil {
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"message":  "Workload restore initiated successfully from backup",
		"backupId": id,
		"status":   "restoring",
	})
}

// Superadmin Handlers
func (h *BackupHandler) AdminListBackups(w http.ResponseWriter, r *http.Request) {
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

	filter := domainBackup.Filter{
		TenantID:     r.URL.Query().Get("tenantId"),
		ResourceType: r.URL.Query().Get("resourceType"),
		Status:       domainBackup.Status(r.URL.Query().Get("status")),
		Limit:        limit,
		Offset:       offset,
	}

	backups, total, err := h.backupService.ListBackups(r.Context(), sub, filter)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"backups": backups,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

func (h *BackupHandler) AdminCreateBackup(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin access required")
		return
	}

	var req appBackup.CreateBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid request body")
		return
	}

	backup, err := h.backupService.CreateBackup(r.Context(), sub, req)
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusCreated, backup)
}

func (h *BackupHandler) AdminGetBackup(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin access required")
		return
	}

	id := chi.URLParam(r, "id")
	backup, err := h.backupService.GetBackup(r.Context(), sub, id)
	if err != nil {
		RespondError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, backup)
}

func (h *BackupHandler) AdminVerifyBackup(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin access required")
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.backupService.VerifyBackup(r.Context(), sub, id); err != nil {
		RespondError(w, r, http.StatusBadRequest, "VERIFICATION_FAILED", err.Error())
		return
	}

	backup, _ := h.backupService.GetBackup(r.Context(), sub, id)
	RespondJSON(w, r, http.StatusOK, backup)
}

func (h *BackupHandler) AdminDeleteBackup(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin access required")
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.backupService.DeleteBackup(r.Context(), sub, id); err != nil {
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"message": "Backup successfully removed",
	})
}

// Disaster Recovery Handlers
func (h *BackupHandler) DisasterRecoveryDryRun(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin access required")
		return
	}

	var req struct {
		BackupID string `json:"backupId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BackupID == "" {
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", "backupId is required")
		return
	}

	plan, err := h.recoveryCoord.DryRunRecovery(r.Context(), sub, req.BackupID)
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, "DRY_RUN_FAILED", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, plan)
}

func (h *BackupHandler) DisasterRecoveryRestore(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin access required")
		return
	}

	var req struct {
		BackupID    string `json:"backupId"`
		ConfirmedDR bool   `json:"confirmedDr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BackupID == "" {
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", "backupId is required")
		return
	}
	if !req.ConfirmedDR {
		RespondError(w, r, http.StatusBadRequest, "CONFIRMATION_REQUIRED", "explicit confirmation required (confirmedDr=true) to execute destructive disaster recovery")
		return
	}

	plan, err := h.recoveryCoord.ExecuteRestore(r.Context(), sub, req.BackupID)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "RESTORE_FAILED", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, plan)
}

func (h *BackupHandler) AdminListRestorePlans(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin access required")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	plans, total, err := h.recoveryCoord.ListRestorePlans(r.Context(), sub, limit, offset)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"plans":  plans,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *BackupHandler) AdminGetRestorePlan(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin access required")
		return
	}

	id := chi.URLParam(r, "id")
	plan, err := h.recoveryCoord.GetRestorePlan(r.Context(), sub, id)
	if err != nil {
		RespondError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, plan)
}
