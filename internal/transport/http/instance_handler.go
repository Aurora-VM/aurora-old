package http

import (
	"encoding/json"
	"errors"
	"net/http"

	appCompute "github.com/aurora-vm/aurora/internal/app/compute"
	appNetwork "github.com/aurora-vm/aurora/internal/app/network"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainNetwork "github.com/aurora-vm/aurora/internal/domain/network"
	"github.com/go-chi/chi/v5"
)

// InstanceHandler handles HTTP REST endpoints for compute instance management and instance-level networking.
type InstanceHandler struct {
	computeService *appCompute.Service
	networkService *appNetwork.Service
	authorizer     identity.Authorizer
}

// NewInstanceHandler constructs a new InstanceHandler.
func NewInstanceHandler(
	computeService *appCompute.Service,
	networkService *appNetwork.Service,
	authorizer identity.Authorizer,
) *InstanceHandler {
	return &InstanceHandler{
		computeService: computeService,
		networkService: networkService,
		authorizer:     authorizer,
	}
}

// RegisterRoutes registers compute instance routes onto the Chi router.
func (h *InstanceHandler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/api/v1/instances", func(r chi.Router) {
		r.Use(authMiddleware)

		// Create Instance
		r.With(RequirePermission(h.authorizer, "instance:create")).
			Post("/", h.CreateInstance)

		// List Instances
		r.With(RequirePermission(h.authorizer, "instance:read")).
			Get("/", h.ListInstances)

		// Get Instance Details
		r.With(RequirePermission(h.authorizer, "instance:read")).
			Get("/{id}", h.GetInstance)

		// Power Control (start, stop, restart)
		r.With(RequirePermission(h.authorizer, "instance:power")).
			Post("/{id}/power", h.PowerAction)

		// Update / Resize Spec
		r.With(RequirePermission(h.authorizer, "instance:update")).
			Patch("/{id}/spec", h.UpdateSpec)

		// Delete Instance
		r.With(RequirePermission(h.authorizer, "instance:delete")).
			Delete("/{id}", h.DeleteInstance)

		// Live Telemetry Metrics
		r.With(RequirePermission(h.authorizer, "instance:read")).
			Get("/{id}/metrics", h.GetMetrics)

		// List Firewall Rules
		r.With(RequirePermission(h.authorizer, "instance:read")).
			Get("/{id}/firewall", h.ListFirewallRules)

		// Replace Firewall Rules
		r.With(RequirePermission(h.authorizer, "instance:update")).
			Put("/{id}/firewall", h.ApplyFirewallRules)

		// Configure Network Interface
		r.With(RequirePermission(h.authorizer, "instance:update")).
			Post("/{id}/network", h.ConfigureNetwork)

		// Guest File Manager
		r.With(RequirePermission(h.authorizer, "instance:files:read")).
			Get("/{id}/files", h.ListFiles)
		r.With(RequirePermission(h.authorizer, "instance:files:write")).
			Post("/{id}/files", h.WriteFile)
		r.With(RequirePermission(h.authorizer, "instance:files:write")).
			Delete("/{id}/files", h.DeleteFile)

		// Instance Backups
		r.With(RequirePermission(h.authorizer, "instance:read")).
			Get("/{id}/backups", h.ListBackups)
		r.With(RequirePermission(h.authorizer, "instance:update")).
			Post("/{id}/backups", h.CreateBackup)
		r.With(RequirePermission(h.authorizer, "instance:update")).
			Post("/{id}/backups/{backupId}/restore", h.RestoreBackup)
		r.With(RequirePermission(h.authorizer, "instance:update")).
			Delete("/{id}/backups/{backupId}", h.DeleteBackup)

		// Instance Snapshots
		r.With(RequirePermission(h.authorizer, "instance:read")).
			Get("/{id}/snapshots", h.ListSnapshots)
		r.With(RequirePermission(h.authorizer, "instance:update")).
			Post("/{id}/snapshots", h.CreateSnapshot)
		r.With(RequirePermission(h.authorizer, "instance:update")).
			Post("/{id}/snapshots/{snapshotId}/restore", h.RestoreSnapshot)
		r.With(RequirePermission(h.authorizer, "instance:update")).
			Delete("/{id}/snapshots/{snapshotId}", h.DeleteSnapshot)
	})
}

func (h *InstanceHandler) CreateInstance(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	var req appCompute.CreateInstanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	inst, err := h.computeService.CreateInstance(r.Context(), sub, req)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInvalidSpec) || errors.Is(err, domainCompute.ErrUnsupportedInstanceType) {
			RespondError(w, r, http.StatusBadRequest, "INVALID_SPEC", err.Error())
			return
		}
		if errors.Is(err, domainCompute.ErrInstanceAlreadyExists) {
			RespondError(w, r, http.StatusConflict, "INSTANCE_ALREADY_EXISTS", "instance with this name already exists")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusCreated, inst)
}

func (h *InstanceHandler) ListInstances(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	instances, err := h.computeService.ListInstances(r.Context(), sub)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, instances)
}

func (h *InstanceHandler) GetInstance(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	inst, err := h.computeService.GetInstance(r.Context(), sub, id)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInstanceNotFound) {
			RespondError(w, r, http.StatusNotFound, "INSTANCE_NOT_FOUND", "instance not found")
			return
		}
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "you do not have access to this instance")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, inst)
}

type PowerRequest struct {
	Action string `json:"action"` // "start", "stop", "restart"
	Force  bool   `json:"force"`
}

func (h *InstanceHandler) PowerAction(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	var req PowerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	inst, err := h.computeService.PowerAction(r.Context(), sub, id, req.Action, req.Force)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInstanceNotFound) {
			RespondError(w, r, http.StatusNotFound, "INSTANCE_NOT_FOUND", "instance not found")
			return
		}
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "you do not have permission to control power for this instance")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, inst)
}

type UpdateSpecRequest struct {
	CPUCores     int   `json:"cpuCores"`
	MemoryBytes  int64 `json:"memoryBytes"`
	StorageBytes int64 `json:"storageBytes"`
}

func (h *InstanceHandler) UpdateSpec(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	var req UpdateSpecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	inst, err := h.computeService.UpdateSpec(r.Context(), sub, id, req.CPUCores, req.MemoryBytes, req.StorageBytes)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInstanceNotFound) {
			RespondError(w, r, http.StatusNotFound, "INSTANCE_NOT_FOUND", "instance not found")
			return
		}
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "you do not have permission to modify this instance")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, inst)
}

func (h *InstanceHandler) DeleteInstance(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	force := r.URL.Query().Get("force") == "true" || r.URL.Query().Get("force") == "1"

	err := h.computeService.DeleteInstance(r.Context(), sub, id, force)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInstanceNotFound) {
			RespondError(w, r, http.StatusNotFound, "INSTANCE_NOT_FOUND", "instance not found")
			return
		}
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "you do not have permission to delete this instance")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"id":      id,
		"deleted": true,
	})
}

func (h *InstanceHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	metrics, err := h.computeService.GetInstanceMetrics(r.Context(), sub, id)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInstanceNotFound) {
			RespondError(w, r, http.StatusNotFound, "INSTANCE_NOT_FOUND", "instance not found")
			return
		}
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "you do not have access to this instance")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, metrics)
}

func (h *InstanceHandler) ListFirewallRules(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	instanceID := chi.URLParam(r, "id")
	rules, err := h.networkService.ListFirewallRules(r.Context(), sub, instanceID)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInstanceNotFound) {
			RespondError(w, r, http.StatusNotFound, "INSTANCE_NOT_FOUND", "instance not found")
			return
		}
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "you do not have access to this instance")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, rules)
}

func (h *InstanceHandler) ApplyFirewallRules(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	instanceID := chi.URLParam(r, "id")
	var req struct {
		Rules []appNetwork.FirewallRuleInput `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	rules, err := h.networkService.ApplyFirewallRules(r.Context(), sub, instanceID, req.Rules)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInstanceNotFound) {
			RespondError(w, r, http.StatusNotFound, "INSTANCE_NOT_FOUND", "instance not found")
			return
		}
		if errors.Is(err, domainNetwork.ErrInvalidProtocol) || errors.Is(err, domainNetwork.ErrInvalidRule) {
			RespondError(w, r, http.StatusBadRequest, "INVALID_RULE", err.Error())
			return
		}
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "you do not have permission to modify this instance")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, rules)
}

func (h *InstanceHandler) ConfigureNetwork(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	instanceID := chi.URLParam(r, "id")
	var req struct {
		InterfaceName string `json:"interfaceName"`
		IPv4Address   string `json:"ipv4Address"`
		IPv4Gateway   string `json:"ipv4Gateway"`
		IPv6Address   string `json:"ipv6Address"`
		IPv6Gateway   string `json:"ipv6Gateway"`
		MACAddress    string `json:"macAddress"`
		VLANID        int    `json:"vlanId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	err := h.networkService.ConfigureNetwork(
		r.Context(), sub, instanceID, req.InterfaceName, req.IPv4Address, req.IPv4Gateway, req.IPv6Address, req.IPv6Gateway, req.MACAddress, req.VLANID,
	)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInstanceNotFound) {
			RespondError(w, r, http.StatusNotFound, "INSTANCE_NOT_FOUND", "instance not found")
			return
		}
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "you do not have permission to modify this instance")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"instanceId": instanceID,
		"configured": true,
	})
}

// ----------------- GUEST FILES -----------------

func (h *InstanceHandler) ListFiles(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	instanceID := chi.URLParam(r, "id")
	targetPath := r.URL.Query().Get("path")
	if targetPath == "" {
		targetPath = "/"
	}

	files, err := h.computeService.ListGuestFiles(r.Context(), sub, instanceID, targetPath)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInstanceNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "instance not found")
			return
		}
		if errors.Is(err, domainCompute.ErrInvalidPath) {
			RespondError(w, r, http.StatusBadRequest, "INVALID_PATH", err.Error())
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"instanceId": instanceID,
		"path":       targetPath,
		"files":      files,
	})
}

func (h *InstanceHandler) WriteFile(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	instanceID := chi.URLParam(r, "id")
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		IsDir   bool   `json:"isDir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "path is required")
		return
	}

	err := h.computeService.WriteGuestFile(r.Context(), sub, instanceID, req.Path, []byte(req.Content), req.IsDir)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInstanceNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "instance not found")
			return
		}
		if errors.Is(err, domainCompute.ErrInvalidPath) || errors.Is(err, domainCompute.ErrFileTooLarge) {
			RespondError(w, r, http.StatusBadRequest, "INVALID_FILE", err.Error())
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusCreated, map[string]interface{}{
		"instanceId": instanceID,
		"path":       req.Path,
		"saved":      true,
	})
}

func (h *InstanceHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	instanceID := chi.URLParam(r, "id")
	targetPath := r.URL.Query().Get("path")
	if targetPath == "" {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "path parameter is required")
		return
	}

	err := h.computeService.DeleteGuestFile(r.Context(), sub, instanceID, targetPath)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInstanceNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "instance not found")
			return
		}
		if errors.Is(err, domainCompute.ErrInvalidPath) {
			RespondError(w, r, http.StatusBadRequest, "INVALID_PATH", err.Error())
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"instanceId": instanceID,
		"path":       targetPath,
		"deleted":    true,
	})
}

// ----------------- BACKUPS -----------------

func (h *InstanceHandler) ListBackups(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	instanceID := chi.URLParam(r, "id")
	backups, err := h.computeService.ListBackups(r.Context(), sub, instanceID)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInstanceNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "instance not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"instanceId": instanceID,
		"backups":    backups,
	})
}

func (h *InstanceHandler) CreateBackup(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	instanceID := chi.URLParam(r, "id")
	var req struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	backup, err := h.computeService.CreateBackup(r.Context(), sub, instanceID, req.Name)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInstanceNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "instance not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusCreated, backup)
}

func (h *InstanceHandler) RestoreBackup(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	instanceID := chi.URLParam(r, "id")
	backupID := chi.URLParam(r, "backupId")

	err := h.computeService.RestoreBackup(r.Context(), sub, instanceID, backupID)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInstanceNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "instance not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"instanceId": instanceID,
		"backupId":   backupID,
		"restored":   true,
	})
}

func (h *InstanceHandler) DeleteBackup(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	instanceID := chi.URLParam(r, "id")
	backupID := chi.URLParam(r, "backupId")

	err := h.computeService.DeleteBackup(r.Context(), sub, instanceID, backupID)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInstanceNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "instance not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"instanceId": instanceID,
		"backupId":   backupID,
		"deleted":    true,
	})
}

// ----------------- SNAPSHOTS -----------------

func (h *InstanceHandler) ListSnapshots(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	instanceID := chi.URLParam(r, "id")
	snapshots, err := h.computeService.ListSnapshots(r.Context(), sub, instanceID)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInstanceNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "instance not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"instanceId": instanceID,
		"snapshots":  snapshots,
	})
}

func (h *InstanceHandler) CreateSnapshot(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	instanceID := chi.URLParam(r, "id")
	var req struct {
		Name     string `json:"name"`
		Stateful bool   `json:"stateful"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	snap, err := h.computeService.CreateSnapshot(r.Context(), sub, instanceID, req.Name, req.Stateful)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInstanceNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "instance not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusCreated, snap)
}

func (h *InstanceHandler) RestoreSnapshot(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	instanceID := chi.URLParam(r, "id")
	snapshotID := chi.URLParam(r, "snapshotId")

	err := h.computeService.RestoreSnapshot(r.Context(), sub, instanceID, snapshotID)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInstanceNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "instance not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"instanceId": instanceID,
		"snapshotId": snapshotID,
		"restored":   true,
	})
}

func (h *InstanceHandler) DeleteSnapshot(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	instanceID := chi.URLParam(r, "id")
	snapshotID := chi.URLParam(r, "snapshotId")

	err := h.computeService.DeleteSnapshot(r.Context(), sub, instanceID, snapshotID)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInstanceNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "instance not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"instanceId": instanceID,
		"snapshotId": snapshotID,
		"deleted":    true,
	})
}
