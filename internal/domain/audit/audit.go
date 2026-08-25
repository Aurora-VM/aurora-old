package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrSIEMDestinationNotFound = errors.New("siem destination not found")
	ErrCorruptedAuditChain     = errors.New("audit log chain integrity verification failed")
	ErrInvalidSIEMSpec         = errors.New("invalid siem destination specification")
)

type Severity string

const (
	SeverityInfo             Severity = "info"
	SeverityWarning          Severity = "warning"
	SeverityCritical         Severity = "critical"
	SeveritySecurityIncident Severity = "security_incident"
)

// AuditLog represents a tamper-evident record of a security or administrative action.
type AuditLog struct {
	ID              int64                  `json:"id"`
	ActorID         *string                `json:"actorId,omitempty"`
	ActorIP         string                 `json:"actorIp,omitempty"`
	UserAgent       string                 `json:"userAgent,omitempty"`
	Action          string                 `json:"action"`       // e.g. "auth.login.success", "instance.create"
	ResourceType    string                 `json:"resourceType"` // e.g. "user", "instance", "volume"
	ResourceID      *string                `json:"resourceId,omitempty"`
	RequestID       string                 `json:"requestId,omitempty"`
	StatusCode      int                    `json:"statusCode,omitempty"`
	Details         map[string]interface{} `json:"details,omitempty"`
	Severity        Severity               `json:"severity"`
	PrevHash        string                 `json:"prevHash"`
	TamperProofHash string                 `json:"tamperProofHash"`
	CreatedAt       time.Time              `json:"createdAt"`
}

// ComputeHash generates an immutable SHA-256 hash linking this log to the previous log.
func (a *AuditLog) ComputeHash() string {
	actor := ""
	if a.ActorID != nil {
		actor = *a.ActorID
	}
	resID := ""
	if a.ResourceID != nil {
		resID = *a.ResourceID
	}
	detailsJSON, _ := json.Marshal(a.Details)

	payload := fmt.Sprintf(
		"%s|%s|%s|%s|%s|%s|%s|%d|%s|%s|%d",
		a.PrevHash,
		actor,
		a.ActorIP,
		a.Action,
		a.ResourceType,
		resID,
		a.RequestID,
		a.StatusCode,
		string(a.Severity),
		string(detailsJSON),
		a.CreatedAt.UnixNano(),
	)

	h := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(h[:])
}

// VerifyHash checks whether the TamperProofHash matches the recorded log contents and PrevHash.
func (a *AuditLog) VerifyHash() bool {
	if a.TamperProofHash == "" {
		return true // Unhashed legacy log
	}
	return a.ComputeHash() == a.TamperProofHash
}

// AuditFilter defines parameters for querying audit trails.
type AuditFilter struct {
	ActorID      string     `json:"actorId,omitempty"`
	Action       string     `json:"action,omitempty"`
	ResourceType string     `json:"resourceType,omitempty"`
	ResourceID   string     `json:"resourceId,omitempty"`
	Severity     Severity   `json:"severity,omitempty"`
	From         *time.Time `json:"from,omitempty"`
	To           *time.Time `json:"to,omitempty"`
	Limit        int        `json:"limit,omitempty"`
	Offset       int        `json:"offset,omitempty"`
}

// Repository defines the interface for recording and querying tamper-evident audit trails.
type Repository interface {
	Record(ctx context.Context, log *AuditLog) error
	ListFiltered(ctx context.Context, filter AuditFilter) ([]*AuditLog, int64, error)
	GetLatestLog(ctx context.Context) (*AuditLog, error)
	VerifyChainIntegrity(ctx context.Context, limit int) (bool, int64, error)
}

type SIEMType string

const (
	SIEMTypeWebhook   SIEMType = "webhook"
	SIEMTypeSyslogTCP SIEMType = "syslog_tcp"
	SIEMTypeSyslogUDP SIEMType = "syslog_udp"
)

type SIEMFormat string

const (
	SIEMFormatJSON    SIEMFormat = "json"
	SIEMFormatCEF     SIEMFormat = "cef"
	SIEMFormatRFC5424 SIEMFormat = "rfc5424"
)

// SIEMDestination represents an external SIEM endpoint for real-time security log streaming.
type SIEMDestination struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Type      SIEMType   `json:"type"`
	Target    string     `json:"target"`
	AuthToken string     `json:"authToken,omitempty"`
	Format    SIEMFormat `json:"format"`
	Enabled   bool       `json:"enabled"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// SIEMRepository defines persistence for configured SIEM forwarders.
type SIEMRepository interface {
	Create(ctx context.Context, dest *SIEMDestination) error
	GetByID(ctx context.Context, id string) (*SIEMDestination, error)
	List(ctx context.Context) ([]*SIEMDestination, error)
	Delete(ctx context.Context, id string) error
}
