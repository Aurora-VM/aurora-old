package audit

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"strconv"
	"time"

	domainAudit "github.com/aurora-vm/aurora/internal/domain/audit"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/aurora-vm/aurora/internal/infra/siem"
)

type ChainVerificationResult struct {
	Valid          bool      `json:"valid"`
	VerifiedCount  int64     `json:"verifiedCount"`
	CorruptedLogID int64     `json:"corruptedLogId,omitempty"`
	CheckedAt      time.Time `json:"checkedAt"`
}

type CreateSIEMRequest struct {
	Name      string                 `json:"name"`
	Type      domainAudit.SIEMType   `json:"type"`
	Target    string                 `json:"target"`
	AuthToken string                 `json:"authToken,omitempty"`
	Format    domainAudit.SIEMFormat `json:"format"`
	Enabled   bool                   `json:"enabled"`
}

// Service manages security audit trail queries, cryptographic verification, and SIEM integrations.
type Service struct {
	auditRepo  domainAudit.Repository
	siemRepo   domainAudit.SIEMRepository
	dispatcher *siem.Dispatcher
	authorizer identity.Authorizer
}

// NewService constructs an Audit and Compliance Service.
func NewService(
	auditRepo domainAudit.Repository,
	siemRepo domainAudit.SIEMRepository,
	dispatcher *siem.Dispatcher,
	authorizer identity.Authorizer,
) *Service {
	return &Service{
		auditRepo:  auditRepo,
		siemRepo:   siemRepo,
		dispatcher: dispatcher,
		authorizer: authorizer,
	}
}

// Record saves an audit log entry and forwards it to SIEM systems.
func (s *Service) Record(ctx context.Context, log *domainAudit.AuditLog) error {
	if log == nil {
		return nil
	}

	if err := s.auditRepo.Record(ctx, log); err != nil {
		return err
	}

	if s.dispatcher != nil {
		s.dispatcher.Dispatch(log)
	}

	return nil
}

func (s *Service) ListFiltered(ctx context.Context, filter domainAudit.AuditFilter) ([]*domainAudit.AuditLog, int64, error) {
	return s.auditRepo.ListFiltered(ctx, filter)
}

func (s *Service) GetLatestLog(ctx context.Context) (*domainAudit.AuditLog, error) {
	return s.auditRepo.GetLatestLog(ctx)
}

func (s *Service) VerifyChainIntegrity(ctx context.Context, limit int) (bool, int64, error) {
	return s.auditRepo.VerifyChainIntegrity(ctx, limit)
}

// ListAuditLogs returns filtered audit entries with tenant boundary scoping.
func (s *Service) ListAuditLogs(
	ctx context.Context,
	sub *identity.Subject,
	filter domainAudit.AuditFilter,
) ([]*domainAudit.AuditLog, int64, error) {
	if err := s.authorizer.Authorize(ctx, sub, "audit:read", nil); err != nil {
		return nil, 0, err
	}

	// Non-admin tenants only see their own audit actions
	if !isSuperadmin(sub) && sub != nil && sub.UserID != "" {
		filter.ActorID = sub.UserID
	}

	return s.auditRepo.ListFiltered(ctx, filter)
}

// VerifyAuditChain validates the SHA-256 hash chaining of audit logs.
func (s *Service) VerifyAuditChain(
	ctx context.Context,
	sub *identity.Subject,
	limit int,
) (*ChainVerificationResult, error) {
	if err := s.authorizer.Authorize(ctx, sub, "audit:read", nil); err != nil {
		return nil, err
	}

	valid, count, err := s.auditRepo.VerifyChainIntegrity(ctx, limit)
	if err != nil {
		return nil, err
	}

	res := &ChainVerificationResult{
		Valid:         valid,
		VerifiedCount: count,
		CheckedAt:     time.Now().UTC(),
	}
	if !valid {
		res.CorruptedLogID = count
	}

	return res, nil
}

// ExportComplianceReport exports audit logs in CSV or JSON format.
func (s *Service) ExportComplianceReport(
	ctx context.Context,
	sub *identity.Subject,
	filter domainAudit.AuditFilter,
	format string,
) ([]byte, string, error) {
	logs, _, err := s.ListAuditLogs(ctx, sub, filter)
	if err != nil {
		return nil, "", err
	}

	if format == "csv" {
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		_ = w.Write([]string{"ID", "Timestamp", "ActorID", "IP", "Action", "ResourceType", "ResourceID", "Status", "Severity", "TamperProofHash"})

		for _, l := range logs {
			actor := ""
			if l.ActorID != nil {
				actor = *l.ActorID
			}
			resID := ""
			if l.ResourceID != nil {
				resID = *l.ResourceID
			}
			_ = w.Write([]string{
				strconv.FormatInt(l.ID, 10),
				l.CreatedAt.Format(time.RFC3339),
				actor,
				l.ActorIP,
				l.Action,
				l.ResourceType,
				resID,
				strconv.Itoa(l.StatusCode),
				string(l.Severity),
				l.TamperProofHash,
			})
		}
		w.Flush()
		return buf.Bytes(), "text/csv", nil
	}

	// Default JSON Export
	data, err := json.MarshalIndent(logs, "", "  ")
	if err != nil {
		return nil, "", err
	}
	return data, "application/json", nil
}

// CreateSIEMDestination registers a new SIEM forwarder.
func (s *Service) CreateSIEMDestination(
	ctx context.Context,
	sub *identity.Subject,
	req CreateSIEMRequest,
) (*domainAudit.SIEMDestination, error) {
	if req.Name == "" || req.Target == "" || req.Type == "" {
		return nil, domainAudit.ErrInvalidSIEMSpec
	}

	if err := s.authorizer.Authorize(ctx, sub, "audit:manage", nil); err != nil {
		return nil, err
	}

	fmtType := req.Format
	if fmtType == "" {
		fmtType = domainAudit.SIEMFormatJSON
	}

	dest := &domainAudit.SIEMDestination{
		Name:      req.Name,
		Type:      req.Type,
		Target:    req.Target,
		AuthToken: req.AuthToken,
		Format:    fmtType,
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.siemRepo.Create(ctx, dest); err != nil {
		return nil, err
	}

	return dest, nil
}

// ListSIEMDestinations returns all registered SIEM endpoints.
func (s *Service) ListSIEMDestinations(
	ctx context.Context,
	sub *identity.Subject,
) ([]*domainAudit.SIEMDestination, error) {
	if err := s.authorizer.Authorize(ctx, sub, "audit:read", nil); err != nil {
		return nil, err
	}
	return s.siemRepo.List(ctx)
}

// DeleteSIEMDestination removes a SIEM endpoint.
func (s *Service) DeleteSIEMDestination(
	ctx context.Context,
	sub *identity.Subject,
	id string,
) error {
	if err := s.authorizer.Authorize(ctx, sub, "audit:manage", nil); err != nil {
		return err
	}
	return s.siemRepo.Delete(ctx, id)
}

func isSuperadmin(sub *identity.Subject) bool {
	if sub == nil {
		return false
	}
	for _, r := range sub.Roles {
		if r == "superadmin" {
			return true
		}
	}
	for _, p := range sub.Permissions {
		if p == "*" {
			return true
		}
	}
	return false
}
