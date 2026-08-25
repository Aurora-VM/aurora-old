package node

import (
	"context"
	"time"
)

// NodeRepository defines the persistence port for enrolled hypervisor nodes.
type NodeRepository interface {
	Create(ctx context.Context, n *Node) error
	GetByID(ctx context.Context, id string) (*Node, error)
	GetByFQDN(ctx context.Context, fqdn string) (*Node, error)
	GetByCertFingerprint(ctx context.Context, fingerprint string) (*Node, error)
	UpdateStatus(ctx context.Context, id string, status Status) error
	UpdateHealthState(ctx context.Context, id string, status Status, reason string) error
	UpdateDrainMode(ctx context.Context, id string, drainMode bool) error
	UpdateHeartbeat(ctx context.Context, id string, lastSeen time.Time, caps map[string]interface{}) error
	UpdateMaintenanceMode(ctx context.Context, id string, inMaintenance bool) error
	Revoke(ctx context.Context, id string) error
	List(ctx context.Context) ([]*Node, error)
}

// EnrollmentRepository defines the persistence port for one-time enrollment tokens.
type EnrollmentRepository interface {
	Create(ctx context.Context, secret *EnrollmentSecret) error
	GetBySecretHash(ctx context.Context, hash string) (*EnrollmentSecret, error)
	MarkUsed(ctx context.Context, id, nodeID string) error
	ListActive(ctx context.Context) ([]*EnrollmentSecret, error)
}

// PKIManager handles cryptographic CA operations, CSR signing, and certificate fingerprint verification.
type PKIManager interface {
	GetCACertificatePEM() []byte
	SignNodeCSR(csrPEM []byte, nodeID, nodeName string, ttl time.Duration) (certPEM []byte, fingerprint string, err error)
	VerifyCertificate(certPEM []byte) (nodeID string, fingerprint string, err error)
	ComputeFingerprint(certPEM []byte) (string, error)
}

// StreamSender abstracts the outbound message transport to a connected node stream.
type StreamSender interface {
	Send(cmd *Command) error
}

// ConnectionManager tracks active live gRPC streams from nodes and manages command dispatching.
type ConnectionManager interface {
	RegisterConnection(nodeID string, sender StreamSender) error
	UnregisterConnection(nodeID string)
	GetConnection(nodeID string) (StreamSender, bool)
	ListConnectedNodeIDs() []string
	DispatchCommand(ctx context.Context, nodeID string, cmd *Command) (*CommandResult, error)
	HandleCommandResult(result *CommandResult)
}
