package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	appNode "github.com/aurora-vm/aurora/internal/app/node"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	"github.com/go-chi/chi/v5"
)

// NodeHandler handles HTTP management endpoints for hypervisor nodes and enrollment tokens.
type NodeHandler struct {
	nodeService *appNode.Service
	authorizer  identity.Authorizer
}

// NewNodeHandler constructs a new NodeHandler.
func NewNodeHandler(nodeService *appNode.Service, authorizer identity.Authorizer) *NodeHandler {
	return &NodeHandler{
		nodeService: nodeService,
		authorizer:  authorizer,
	}
}

// RegisterRoutes registers node management routes onto the Chi router.
func (h *NodeHandler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	// Public Node Enrollment Endpoint (authenticated via one-time enrollment token)
	r.Post("/api/v1/nodes/enroll", h.EnrollNode)

	r.Route("/api/v1/nodes", func(r chi.Router) {
		r.Use(authMiddleware)

		// Create Enrollment Token
		r.With(RequirePermission(h.authorizer, "node:create")).
			Post("/enrollment-tokens", h.CreateEnrollmentToken)

		// List Nodes
		r.With(RequirePermission(h.authorizer, "node:read")).
			Get("/", h.ListNodes)

		// Get Node Details
		r.With(RequirePermission(h.authorizer, "node:read")).
			Get("/{id}", h.GetNode)

		// Toggle Maintenance Mode
		r.With(RequirePermission(h.authorizer, "node:maintenance")).
			Post("/{id}/maintenance", h.ToggleMaintenance)

		// Toggle Drain Mode
		r.With(RequirePermission(h.authorizer, "node:drain")).
			Post("/{id}/drain", h.DrainNode)
		r.With(RequirePermission(h.authorizer, "node:drain")).
			Post("/{id}/undrain", h.UndrainNode)

		// Revoke Node
		r.With(RequirePermission(h.authorizer, "node:update")).
			Post("/{id}/revoke", h.RevokeNode)

		// Test Ping Command
		r.With(RequirePermission(h.authorizer, "node:read")).
			Post("/{id}/commands/ping", h.PingNode)
	})
}

type EnrollRequest struct {
	EnrollmentToken string                 `json:"enrollmentToken"`
	NodeName        string                 `json:"nodeName"`
	FQDN            string                 `json:"fqdn"`
	CSRPEM          string                 `json:"csrPem"`
	Capabilities    map[string]interface{} `json:"capabilities,omitempty"`
}

func (h *NodeHandler) EnrollNode(w http.ResponseWriter, r *http.Request) {
	var req EnrollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	if req.EnrollmentToken == "" || req.CSRPEM == "" {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "enrollmentToken and csrPem are required")
		return
	}

	nodeID, certPEM, caCertPEM, interval, err := h.nodeService.EnrollNode(
		r.Context(), req.EnrollmentToken, req.NodeName, req.FQDN, []byte(req.CSRPEM), req.Capabilities,
	)
	if err != nil {
		if errors.Is(err, domainNode.ErrEnrollmentTokenInvalid) {
			RespondError(w, r, http.StatusUnauthorized, "ENROLLMENT_TOKEN_INVALID", "invalid enrollment token")
			return
		}
		if errors.Is(err, domainNode.ErrEnrollmentTokenExpired) {
			RespondError(w, r, http.StatusUnauthorized, "ENROLLMENT_TOKEN_EXPIRED", "enrollment token has expired")
			return
		}
		if errors.Is(err, domainNode.ErrEnrollmentTokenUsed) {
			RespondError(w, r, http.StatusConflict, "ENROLLMENT_TOKEN_USED", "enrollment token has already been consumed")
			return
		}
		if errors.Is(err, domainNode.ErrInvalidCSR) {
			RespondError(w, r, http.StatusBadRequest, "INVALID_CSR", err.Error())
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"nodeId":                   nodeID,
		"certificatePem":           string(certPEM),
		"caCertificatePem":         string(caCertPEM),
		"heartbeatIntervalSeconds": interval,
	})
}

type CreateTokenRequest struct {
	LocationID      string `json:"locationId"`
	NodeNamePattern string `json:"nodeNamePattern,omitempty"`
	TTLSeconds      int    `json:"ttlSeconds,omitempty"`
}

func (h *NodeHandler) CreateEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	var req CreateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	ttl := 1 * time.Hour
	if req.TTLSeconds > 0 {
		ttl = time.Duration(req.TTLSeconds) * time.Second
	}

	var creatorID *string
	if sub := GetSubject(r.Context()); sub != nil {
		creatorID = &sub.UserID
	}

	token, secret, err := h.nodeService.CreateEnrollmentToken(r.Context(), req.LocationID, req.NodeNamePattern, ttl, creatorID)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusCreated, map[string]interface{}{
		"enrollmentToken": token,
		"secret":          secret,
	})
}

func (h *NodeHandler) ListNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.nodeService.ListNodes(r.Context())
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	RespondJSON(w, r, http.StatusOK, nodes)
}

func (h *NodeHandler) GetNode(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "id")
	n, err := h.nodeService.GetNode(r.Context(), nodeID)
	if err != nil {
		if errors.Is(err, domainNode.ErrNodeNotFound) {
			RespondError(w, r, http.StatusNotFound, "NODE_NOT_FOUND", "node not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	RespondJSON(w, r, http.StatusOK, n)
}

type MaintenanceRequest struct {
	Enabled bool `json:"enabled"`
}

func (h *NodeHandler) ToggleMaintenance(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "id")
	var req MaintenanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	err := h.nodeService.ToggleMaintenance(r.Context(), nodeID, req.Enabled)
	if err != nil {
		if errors.Is(err, domainNode.ErrNodeNotFound) {
			RespondError(w, r, http.StatusNotFound, "NODE_NOT_FOUND", "node not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"nodeId":          nodeID,
		"maintenanceMode": req.Enabled,
	})
}

func (h *NodeHandler) DrainNode(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "id")
	err := h.nodeService.ToggleDrainMode(r.Context(), nodeID, true)
	if err != nil {
		if errors.Is(err, domainNode.ErrNodeNotFound) {
			RespondError(w, r, http.StatusNotFound, "NODE_NOT_FOUND", "node not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"nodeId":    nodeID,
		"drainMode": true,
	})
}

func (h *NodeHandler) UndrainNode(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "id")
	err := h.nodeService.ToggleDrainMode(r.Context(), nodeID, false)
	if err != nil {
		if errors.Is(err, domainNode.ErrNodeNotFound) {
			RespondError(w, r, http.StatusNotFound, "NODE_NOT_FOUND", "node not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"nodeId":    nodeID,
		"drainMode": false,
	})
}

func (h *NodeHandler) RevokeNode(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "id")
	err := h.nodeService.RevokeNode(r.Context(), nodeID)
	if err != nil {
		if errors.Is(err, domainNode.ErrNodeNotFound) {
			RespondError(w, r, http.StatusNotFound, "NODE_NOT_FOUND", "node not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"nodeId":  nodeID,
		"revoked": true,
	})
}

func (h *NodeHandler) PingNode(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "id")
	cmdCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	cmd := &domainNode.Command{
		Type:    "ping",
		Payload: map[string]interface{}{"msg": "health_ping"},
	}

	res, err := h.nodeService.SendCommand(cmdCtx, nodeID, cmd)
	if err != nil {
		if errors.Is(err, domainNode.ErrNodeOffline) {
			RespondError(w, r, http.StatusServiceUnavailable, "NODE_OFFLINE", "node is currently offline")
			return
		}
		if errors.Is(err, domainNode.ErrNodeRevoked) {
			RespondError(w, r, http.StatusForbidden, "NODE_REVOKED", "node is revoked")
			return
		}
		RespondError(w, r, http.StatusGatewayTimeout, "COMMAND_FAILED", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, res)
}
