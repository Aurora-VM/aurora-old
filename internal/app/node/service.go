package node

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/audit"
	"github.com/aurora-vm/aurora/internal/domain/node"
	"github.com/google/uuid"
)

// Service coordinates node enrollment, mTLS validation, heartbeat tracking, and command execution.
type Service struct {
	nodeRepo        node.NodeRepository
	enrollRepo      node.EnrollmentRepository
	pkiManager      node.PKIManager
	connManager     node.ConnectionManager
	auditRepo       audit.Repository
	hubGRPCEndpoint string
}

// NewService constructs a Node Application Service.
func NewService(
	nodeRepo node.NodeRepository,
	enrollRepo node.EnrollmentRepository,
	pkiManager node.PKIManager,
	connManager node.ConnectionManager,
	auditRepo audit.Repository,
	hubGRPCEndpoint string,
) *Service {
	if hubGRPCEndpoint == "" {
		hubGRPCEndpoint = "127.0.0.1:8443"
	}
	return &Service{
		nodeRepo:        nodeRepo,
		enrollRepo:      enrollRepo,
		pkiManager:      pkiManager,
		connManager:     connManager,
		auditRepo:       auditRepo,
		hubGRPCEndpoint: hubGRPCEndpoint,
	}
}

// CreateEnrollmentToken generates a short-lived one-time enrollment secret.
func (s *Service) CreateEnrollmentToken(
	ctx context.Context, locationID, namePattern string, ttl time.Duration, createdBy *string,
) (string, *node.EnrollmentSecret, error) {
	if ttl == 0 {
		ttl = 1 * time.Hour // Default 1 hour TTL
	}

	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return "", nil, fmt.Errorf("failed to generate random token: %w", err)
	}

	plaintextToken := "aur_enroll_" + hex.EncodeToString(rawBytes)
	tokenHash := s.hashToken(plaintextToken)

	secret := &node.EnrollmentSecret{
		ID:              uuid.New().String(),
		LocationID:      locationID,
		SecretHash:      tokenHash,
		NodeNamePattern: namePattern,
		ExpiresAt:       time.Now().Add(ttl).UTC(),
		CreatedBy:       createdBy,
		CreatedAt:       time.Now().UTC(),
	}

	if err := s.enrollRepo.Create(ctx, secret); err != nil {
		return "", nil, fmt.Errorf("failed to save enrollment secret: %w", err)
	}

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      createdBy,
		Action:       "node.enrollment_token.created",
		ResourceType: "enrollment_secret",
		ResourceID:   &secret.ID,
		StatusCode:   201,
		Details:      map[string]interface{}{"locationId": locationID, "expiresAt": secret.ExpiresAt},
		CreatedAt:    time.Now().UTC(),
	})

	return plaintextToken, secret, nil
}

// EnrollNode exchanges a one-time enrollment token + CSR for a signed X.509 client certificate.
func (s *Service) EnrollNode(
	ctx context.Context, tokenPlaintext, nodeName, fqdn string, csrPEM []byte, capabilities map[string]interface{},
) (string, []byte, []byte, int, error) {
	tokenHash := s.hashToken(tokenPlaintext)
	secret, err := s.enrollRepo.GetBySecretHash(ctx, tokenHash)
	if err != nil {
		return "", nil, nil, 0, node.ErrEnrollmentTokenInvalid
	}

	if !secret.IsValid() {
		if secret.UsedAt != nil {
			return "", nil, nil, 0, node.ErrEnrollmentTokenUsed
		}
		return "", nil, nil, 0, node.ErrEnrollmentTokenExpired
	}

	// Sign CSR via PKI Manager
	nodeID := uuid.New().String()
	certPEM, fingerprint, err := s.pkiManager.SignNodeCSR(csrPEM, nodeID, fqdn, 90*24*time.Hour)
	if err != nil {
		return "", nil, nil, 0, fmt.Errorf("%w: %v", node.ErrInvalidCSR, err)
	}

	// Create or initialize Node entity
	newNode := &node.Node{
		ID:              nodeID,
		LocationID:      secret.LocationID,
		Name:            nodeName,
		FQDN:            fqdn,
		Status:          node.StatusEnrolling,
		CertFingerprint: fingerprint,
		Capabilities:    capabilities,
		MaintenanceMode: false,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	if err := s.nodeRepo.Create(ctx, newNode); err != nil {
		return "", nil, nil, 0, fmt.Errorf("failed to register node record: %w", err)
	}

	// Mark enrollment token as used
	if err := s.enrollRepo.MarkUsed(ctx, secret.ID, nodeID); err != nil {
		return "", nil, nil, 0, fmt.Errorf("failed to mark token as used: %w", err)
	}

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		Action:       "node.enrolled",
		ResourceType: "node",
		ResourceID:   &nodeID,
		StatusCode:   201,
		Details:      map[string]interface{}{"name": nodeName, "fqdn": fqdn, "fingerprint": fingerprint},
		CreatedAt:    time.Now().UTC(),
	})

	caCertPEM := s.pkiManager.GetCACertificatePEM()
	heartbeatIntervalSeconds := 10

	return nodeID, certPEM, caCertPEM, heartbeatIntervalSeconds, nil
}

// AuthenticateNodeCertificate validates a client certificate against the PKI Root CA and node registry.
func (s *Service) AuthenticateNodeCertificate(ctx context.Context, certPEM []byte) (*node.Node, error) {
	nodeID, fingerprint, err := s.pkiManager.VerifyCertificate(certPEM)
	if err != nil {
		return nil, fmt.Errorf("invalid client certificate: %w", err)
	}

	n, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, node.ErrNodeNotFound
	}

	if n.Status == node.StatusRevoked {
		return nil, node.ErrNodeRevoked
	}

	if n.CertFingerprint != fingerprint {
		return nil, node.ErrCertMismatch
	}

	return n, nil
}

// OnStreamConnected transitions node to online and registers its stream.
func (s *Service) OnStreamConnected(ctx context.Context, nodeID string, sender node.StreamSender) error {
	n, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return err
	}
	if n.Status == node.StatusRevoked {
		return node.ErrNodeRevoked
	}

	if err := s.connManager.RegisterConnection(nodeID, sender); err != nil {
		return err
	}

	newStatus := node.StatusOnline
	if n.MaintenanceMode {
		newStatus = node.StatusMaintenance
	}
	_ = s.nodeRepo.UpdateStatus(ctx, nodeID, newStatus)

	if s.auditRepo != nil {
		_ = s.auditRepo.Record(ctx, &audit.AuditLog{
			Action:       "node.gateway.connected",
			ResourceType: "node",
			ResourceID:   &nodeID,
			StatusCode:   200,
			CreatedAt:    time.Now().UTC(),
		})
	}

	return nil
}

// OnStreamDisconnected transitions node to offline and removes active connection.
func (s *Service) OnStreamDisconnected(ctx context.Context, nodeID string) {
	s.connManager.UnregisterConnection(nodeID)

	n, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err == nil && n.Status != node.StatusRevoked {
		_ = s.nodeRepo.UpdateStatus(ctx, nodeID, node.StatusOffline)
	}

	if s.auditRepo != nil {
		_ = s.auditRepo.Record(ctx, &audit.AuditLog{
			Action:       "node.gateway.disconnected",
			ResourceType: "node",
			ResourceID:   &nodeID,
			StatusCode:   200,
			CreatedAt:    time.Now().UTC(),
		})
	}
}

// ProcessHeartbeat records heartbeat timestamps and capability updates.
func (s *Service) ProcessHeartbeat(ctx context.Context, nodeID string, caps map[string]interface{}) error {
	n, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return err
	}
	if n.Status == node.StatusRevoked {
		return node.ErrNodeRevoked
	}

	return s.nodeRepo.UpdateHeartbeat(ctx, nodeID, time.Now().UTC(), caps)
}

// SendCommand dispatches a typed command to a connected node.
func (s *Service) SendCommand(ctx context.Context, nodeID string, cmd *node.Command) (*node.CommandResult, error) {
	n, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if n.Status == node.StatusRevoked {
		return nil, node.ErrNodeRevoked
	}

	return s.connManager.DispatchCommand(ctx, nodeID, cmd)
}

// HandleCommandResult routes incoming results to waiting command dispatchers.
func (s *Service) HandleCommandResult(result *node.CommandResult) {
	s.connManager.HandleCommandResult(result)
}

// ListNodes returns all registered nodes.
func (s *Service) ListNodes(ctx context.Context) ([]*node.Node, error) {
	return s.nodeRepo.List(ctx)
}

// GetNode returns details for a specific node.
func (s *Service) GetNode(ctx context.Context, nodeID string) (*node.Node, error) {
	return s.nodeRepo.GetByID(ctx, nodeID)
}

// ToggleMaintenance enables or disables maintenance mode on a node.
func (s *Service) ToggleMaintenance(ctx context.Context, nodeID string, inMaintenance bool) error {
	n, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return err
	}
	if n.Status == node.StatusRevoked {
		return node.ErrNodeRevoked
	}

	err = s.nodeRepo.UpdateMaintenanceMode(ctx, nodeID, inMaintenance)
	if err != nil {
		return err
	}

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		Action:       "node.maintenance.updated",
		ResourceType: "node",
		ResourceID:   &nodeID,
		StatusCode:   200,
		Details:      map[string]interface{}{"maintenanceMode": inMaintenance},
		CreatedAt:    time.Now().UTC(),
	})

	return nil
}

// ToggleDrainMode sets or clears the drain mode flag on a node.
func (s *Service) ToggleDrainMode(ctx context.Context, nodeID string, drain bool) error {
	n, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return err
	}
	if n.Status == node.StatusRevoked {
		return node.ErrNodeRevoked
	}

	err = s.nodeRepo.UpdateDrainMode(ctx, nodeID, drain)
	if err != nil {
		return err
	}

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		Action:       "node.drain.updated",
		ResourceType: "node",
		ResourceID:   &nodeID,
		StatusCode:   200,
		Details:      map[string]interface{}{"drainMode": drain},
		CreatedAt:    time.Now().UTC(),
	})

	return nil
}

// RevokeNode permanently revokes a node, terminates its connection, and marks it as revoked.
func (s *Service) RevokeNode(ctx context.Context, nodeID string) error {
	if err := s.nodeRepo.Revoke(ctx, nodeID); err != nil {
		return err
	}

	// Terminate active connection if connected
	s.connManager.UnregisterConnection(nodeID)

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		Action:       "node.revoked",
		ResourceType: "node",
		ResourceID:   &nodeID,
		StatusCode:   200,
		CreatedAt:    time.Now().UTC(),
	})

	return nil
}

func (s *Service) GetConnection(nodeID string) (node.StreamSender, bool) {
	if s.connManager == nil {
		return nil, false
	}
	return s.connManager.GetConnection(nodeID)
}

func (s *Service) hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
