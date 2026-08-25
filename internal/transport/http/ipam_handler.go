package http

import (
	"encoding/json"
	"errors"
	"net/http"

	appIPAM "github.com/aurora-vm/aurora/internal/app/ipam"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainIPAM "github.com/aurora-vm/aurora/internal/domain/ipam"
	"github.com/go-chi/chi/v5"
)

// IPAMHandler handles REST API endpoints for IP address management and pools.
type IPAMHandler struct {
	ipamService *appIPAM.Service
	authorizer  identity.Authorizer
}

// NewIPAMHandler constructs an IPAMHandler.
func NewIPAMHandler(ipamService *appIPAM.Service, authorizer identity.Authorizer) *IPAMHandler {
	return &IPAMHandler{
		ipamService: ipamService,
		authorizer:  authorizer,
	}
}

// RegisterRoutes registers IPAM endpoints onto the Chi router.
func (h *IPAMHandler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/api/v1/ipam", func(r chi.Router) {
		r.Use(authMiddleware)

		// Create IP Pool
		r.With(RequirePermission(h.authorizer, "ipam:manage")).
			Post("/pools", h.CreatePool)

		// List IP Pools
		r.With(RequirePermission(h.authorizer, "ipam:read")).
			Get("/pools", h.ListPools)

		// Get Pool Details & Utilization
		r.With(RequirePermission(h.authorizer, "ipam:read")).
			Get("/pools/{id}", h.GetPool)

		// List Pool Allocations
		r.With(RequirePermission(h.authorizer, "ipam:read")).
			Get("/pools/{id}/allocations", h.ListAllocations)

		// Allocate IP Address
		r.With(RequirePermission(h.authorizer, "ipam:manage")).
			Post("/pools/{id}/allocate", h.AllocateIP)

		// Release IP Address
		r.With(RequirePermission(h.authorizer, "ipam:manage")).
			Delete("/allocations/{id}", h.ReleaseIP)
	})
}

func (h *IPAMHandler) CreatePool(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	var req appIPAM.CreatePoolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	pool, err := h.ipamService.CreatePool(r.Context(), sub, req)
	if err != nil {
		if errors.Is(err, domainIPAM.ErrInvalidCIDR) {
			RespondError(w, r, http.StatusBadRequest, "INVALID_CIDR", err.Error())
			return
		}
		if errors.Is(err, domainIPAM.ErrIPPoolAlreadyExists) {
			RespondError(w, r, http.StatusConflict, "POOL_ALREADY_EXISTS", "pool with this CIDR already exists")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusCreated, pool)
}

func (h *IPAMHandler) ListPools(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	locationID := r.URL.Query().Get("locationId")
	pools, err := h.ipamService.ListPools(r.Context(), sub, locationID)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, pools)
}

func (h *IPAMHandler) GetPool(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	pool, util, err := h.ipamService.GetPool(r.Context(), sub, id)
	if err != nil {
		if errors.Is(err, domainIPAM.ErrIPPoolNotFound) {
			RespondError(w, r, http.StatusNotFound, "POOL_NOT_FOUND", "ip pool not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"pool":        pool,
		"utilization": util,
	})
}

type AllocateIPRequest struct {
	InstanceID    *string `json:"instanceId,omitempty"`
	InterfaceName string  `json:"interfaceName"`
	IsReserved    bool    `json:"isReserved"`
	Notes         string  `json:"notes,omitempty"`
}

func (h *IPAMHandler) AllocateIP(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	poolID := chi.URLParam(r, "id")
	var req AllocateIPRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	alloc, err := h.ipamService.AllocateIP(r.Context(), sub, poolID, req.InstanceID, req.InterfaceName, req.IsReserved, req.Notes)
	if err != nil {
		if errors.Is(err, domainIPAM.ErrIPPoolNotFound) {
			RespondError(w, r, http.StatusNotFound, "POOL_NOT_FOUND", "ip pool not found")
			return
		}
		if errors.Is(err, domainIPAM.ErrIPPoolExhausted) {
			RespondError(w, r, http.StatusConflict, "POOL_EXHAUSTED", "no available IP addresses in pool")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusCreated, alloc)
}

func (h *IPAMHandler) ReleaseIP(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	err := h.ipamService.ReleaseIP(r.Context(), sub, id)
	if err != nil {
		if errors.Is(err, domainIPAM.ErrIPAllocationNotFound) {
			RespondError(w, r, http.StatusNotFound, "ALLOCATION_NOT_FOUND", "ip allocation not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"id":       id,
		"released": true,
	})
}

func (h *IPAMHandler) ListAllocations(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	poolID := chi.URLParam(r, "id")
	allocations, err := h.ipamService.ListAllocations(r.Context(), sub, poolID)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, allocations)
}
