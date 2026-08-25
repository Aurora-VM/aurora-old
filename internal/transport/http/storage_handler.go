package http

import (
	"encoding/json"
	"errors"
	"net/http"

	appStorage "github.com/aurora-vm/aurora/internal/app/storage"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainStorage "github.com/aurora-vm/aurora/internal/domain/storage"
	"github.com/go-chi/chi/v5"
)

// StorageHandler handles REST API endpoints for storage pools, volumes, and snapshots.
type StorageHandler struct {
	storageService *appStorage.Service
	authorizer     identity.Authorizer
}

// NewStorageHandler constructs a StorageHandler.
func NewStorageHandler(storageService *appStorage.Service, authorizer identity.Authorizer) *StorageHandler {
	return &StorageHandler{
		storageService: storageService,
		authorizer:     authorizer,
	}
}

// RegisterRoutes registers storage routes onto the Chi router.
func (h *StorageHandler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	// Storage Pool Routes
	r.Route("/api/v1/storage/pools", func(r chi.Router) {
		r.Use(authMiddleware)

		// Create Storage Pool
		r.With(RequirePermission(h.authorizer, "storage:manage")).
			Post("/", h.CreateStoragePool)

		// List Storage Pools
		r.With(RequirePermission(h.authorizer, "storage:read")).
			Get("/", h.ListStoragePools)

		// Get Storage Pool Details
		r.With(RequirePermission(h.authorizer, "storage:read")).
			Get("/{id}", h.GetStoragePool)
	})

	// Volume Routes
	r.Route("/api/v1/volumes", func(r chi.Router) {
		r.Use(authMiddleware)

		// Create Volume
		r.With(RequirePermission(h.authorizer, "volume:create")).
			Post("/", h.CreateVolume)

		// List Volumes
		r.With(RequirePermission(h.authorizer, "volume:read")).
			Get("/", h.ListVolumes)

		// Get Volume Details
		r.With(RequirePermission(h.authorizer, "volume:read")).
			Get("/{id}", h.GetVolume)

		// Attach Volume
		r.With(RequirePermission(h.authorizer, "volume:attach")).
			Post("/{id}/attach", h.AttachVolume)

		// Detach Volume
		r.With(RequirePermission(h.authorizer, "volume:detach")).
			Post("/{id}/detach", h.DetachVolume)

		// Resize Volume
		r.With(RequirePermission(h.authorizer, "volume:update")).
			Patch("/{id}/resize", h.ResizeVolume)

		// Delete Volume
		r.With(RequirePermission(h.authorizer, "volume:delete")).
			Delete("/{id}", h.DeleteVolume)

		// Volume Snapshots
		r.With(RequirePermission(h.authorizer, "volume:snapshot")).
			Post("/{id}/snapshots", h.CreateSnapshot)

		r.With(RequirePermission(h.authorizer, "volume:read")).
			Get("/{id}/snapshots", h.ListSnapshots)

		r.With(RequirePermission(h.authorizer, "volume:restore")).
			Post("/{id}/snapshots/{snapshotId}/restore", h.RestoreSnapshot)

		r.With(RequirePermission(h.authorizer, "volume:snapshot")).
			Delete("/{id}/snapshots/{snapshotId}", h.DeleteSnapshot)
	})
}

func (h *StorageHandler) CreateStoragePool(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	var req appStorage.CreateStoragePoolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	pool, err := h.storageService.CreateStoragePool(r.Context(), sub, req)
	if err != nil {
		if errors.Is(err, domainStorage.ErrInvalidStoragePoolSpec) {
			RespondError(w, r, http.StatusBadRequest, "INVALID_SPEC", err.Error())
			return
		}
		if errors.Is(err, domainStorage.ErrStoragePoolAlreadyExists) {
			RespondError(w, r, http.StatusConflict, "POOL_ALREADY_EXISTS", "pool with this name already exists on node")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusCreated, pool)
}

func (h *StorageHandler) ListStoragePools(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	nodeID := r.URL.Query().Get("nodeId")
	pools, err := h.storageService.ListStoragePools(r.Context(), sub, nodeID)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, pools)
}

func (h *StorageHandler) GetStoragePool(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	pool, err := h.storageService.GetStoragePool(r.Context(), sub, id)
	if err != nil {
		if errors.Is(err, domainStorage.ErrStoragePoolNotFound) {
			RespondError(w, r, http.StatusNotFound, "POOL_NOT_FOUND", "storage pool not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, pool)
}

func (h *StorageHandler) CreateVolume(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	var req appStorage.CreateVolumeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	vol, err := h.storageService.CreateVolume(r.Context(), sub, req)
	if err != nil {
		if errors.Is(err, domainStorage.ErrInvalidVolumeSpec) {
			RespondError(w, r, http.StatusBadRequest, "INVALID_SPEC", err.Error())
			return
		}
		if errors.Is(err, domainStorage.ErrStoragePoolNotFound) {
			RespondError(w, r, http.StatusNotFound, "POOL_NOT_FOUND", "storage pool not found")
			return
		}
		if errors.Is(err, domainStorage.ErrVolumeAlreadyExists) {
			RespondError(w, r, http.StatusConflict, "VOLUME_ALREADY_EXISTS", "volume with this name already exists in pool")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusCreated, vol)
}

func (h *StorageHandler) ListVolumes(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	poolID := r.URL.Query().Get("poolId")
	instanceID := r.URL.Query().Get("instanceId")

	volumes, err := h.storageService.ListVolumes(r.Context(), sub, poolID, instanceID)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, volumes)
}

func (h *StorageHandler) GetVolume(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	vol, err := h.storageService.GetVolume(r.Context(), sub, id)
	if err != nil {
		if errors.Is(err, domainStorage.ErrVolumeNotFound) {
			RespondError(w, r, http.StatusNotFound, "VOLUME_NOT_FOUND", "volume not found")
			return
		}
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "you do not have access to this volume")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, vol)
}

type AttachVolumeRequest struct {
	InstanceID string `json:"instanceId"`
	MountPath  string `json:"mountPath,omitempty"`
	ReadOnly   bool   `json:"readOnly,omitempty"`
}

func (h *StorageHandler) AttachVolume(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	var req AttachVolumeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	vol, err := h.storageService.AttachVolume(r.Context(), sub, id, req.InstanceID, req.MountPath, req.ReadOnly)
	if err != nil {
		if errors.Is(err, domainStorage.ErrVolumeNotFound) {
			RespondError(w, r, http.StatusNotFound, "VOLUME_NOT_FOUND", "volume not found")
			return
		}
		if errors.Is(err, domainStorage.ErrVolumeAttached) {
			RespondError(w, r, http.StatusConflict, "VOLUME_ATTACHED", "volume is already attached to an instance")
			return
		}
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "you do not have permission to attach this volume")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, vol)
}

func (h *StorageHandler) DetachVolume(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	vol, err := h.storageService.DetachVolume(r.Context(), sub, id)
	if err != nil {
		if errors.Is(err, domainStorage.ErrVolumeNotFound) {
			RespondError(w, r, http.StatusNotFound, "VOLUME_NOT_FOUND", "volume not found")
			return
		}
		if errors.Is(err, domainStorage.ErrVolumeNotAttached) {
			RespondError(w, r, http.StatusBadRequest, "VOLUME_NOT_ATTACHED", "volume is not attached")
			return
		}
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "you do not have permission to detach this volume")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, vol)
}

type ResizeVolumeRequest struct {
	SizeBytes int64 `json:"sizeBytes"`
}

func (h *StorageHandler) ResizeVolume(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	var req ResizeVolumeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	vol, err := h.storageService.ResizeVolume(r.Context(), sub, id, req.SizeBytes)
	if err != nil {
		if errors.Is(err, domainStorage.ErrVolumeNotFound) {
			RespondError(w, r, http.StatusNotFound, "VOLUME_NOT_FOUND", "volume not found")
			return
		}
		if errors.Is(err, domainStorage.ErrVolumeResizeDownNotAllowed) {
			RespondError(w, r, http.StatusBadRequest, "INVALID_SIZE", "shrinking volume is not supported")
			return
		}
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "you do not have permission to resize this volume")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, vol)
}

func (h *StorageHandler) DeleteVolume(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	err := h.storageService.DeleteVolume(r.Context(), sub, id)
	if err != nil {
		if errors.Is(err, domainStorage.ErrVolumeNotFound) {
			RespondError(w, r, http.StatusNotFound, "VOLUME_NOT_FOUND", "volume not found")
			return
		}
		if errors.Is(err, domainStorage.ErrVolumeAttached) {
			RespondError(w, r, http.StatusConflict, "VOLUME_ATTACHED", "cannot delete volume while attached to an instance")
			return
		}
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "you do not have permission to delete this volume")
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

type CreateSnapshotRequest struct {
	Name string `json:"name"`
}

func (h *StorageHandler) CreateSnapshot(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	var req CreateSnapshotRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	snap, err := h.storageService.CreateSnapshot(r.Context(), sub, id, req.Name)
	if err != nil {
		if errors.Is(err, domainStorage.ErrVolumeNotFound) {
			RespondError(w, r, http.StatusNotFound, "VOLUME_NOT_FOUND", "volume not found")
			return
		}
		if errors.Is(err, domainStorage.ErrVolumeSnapshotAlreadyExists) {
			RespondError(w, r, http.StatusConflict, "SNAPSHOT_ALREADY_EXISTS", "snapshot with this name already exists")
			return
		}
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "you do not have permission to snapshot this volume")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusCreated, snap)
}

func (h *StorageHandler) ListSnapshots(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	snaps, err := h.storageService.ListSnapshots(r.Context(), sub, id)
	if err != nil {
		if errors.Is(err, domainStorage.ErrVolumeNotFound) {
			RespondError(w, r, http.StatusNotFound, "VOLUME_NOT_FOUND", "volume not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, snaps)
}

func (h *StorageHandler) RestoreSnapshot(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	volumeID := chi.URLParam(r, "id")
	snapshotID := chi.URLParam(r, "snapshotId")

	err := h.storageService.RestoreSnapshot(r.Context(), sub, volumeID, snapshotID)
	if err != nil {
		if errors.Is(err, domainStorage.ErrVolumeNotFound) || errors.Is(err, domainStorage.ErrVolumeSnapshotNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "volume or snapshot not found")
			return
		}
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "you do not have permission to restore this snapshot")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"volumeId":   volumeID,
		"snapshotId": snapshotID,
		"restored":   true,
	})
}

func (h *StorageHandler) DeleteSnapshot(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	volumeID := chi.URLParam(r, "id")
	snapshotID := chi.URLParam(r, "snapshotId")

	err := h.storageService.DeleteSnapshot(r.Context(), sub, volumeID, snapshotID)
	if err != nil {
		if errors.Is(err, domainStorage.ErrVolumeNotFound) || errors.Is(err, domainStorage.ErrVolumeSnapshotNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "volume or snapshot not found")
			return
		}
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "you do not have permission to delete this snapshot")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"volumeId":   volumeID,
		"snapshotId": snapshotID,
		"deleted":    true,
	})
}
