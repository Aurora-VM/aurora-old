package webhook

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/audit"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainWebhook "github.com/aurora-vm/aurora/internal/domain/webhook"
	"github.com/aurora-vm/aurora/internal/infra/ssrf"
	"github.com/google/uuid"
)

var (
	ErrUnauthorizedTenant = errors.New("unauthorized tenant access to webhook")
)

type CreateWebhookInput struct {
	Name        string   `json:"name"`
	URL         string   `json:"url"`
	Description string   `json:"description,omitempty"`
	EventTypes  []string `json:"eventTypes"`
	Active      *bool    `json:"active,omitempty"`
}

type UpdateWebhookInput struct {
	Name        *string  `json:"name,omitempty"`
	URL         *string  `json:"url,omitempty"`
	Description *string  `json:"description,omitempty"`
	EventTypes  []string `json:"eventTypes,omitempty"`
	Active      *bool    `json:"active,omitempty"`
}

// WebhookCreatedResponse returns the created endpoint along with the secret revealed once.
type WebhookCreatedResponse struct {
	Endpoint *domainWebhook.WebhookEndpoint `json:"endpoint"`
	Secret   string                         `json:"secret"` // Shown ONLY on creation or rotation
}

// WebhookTester executes test pings for webhooks.
type WebhookTester interface {
	TestWebhook(ctx context.Context, endpoint *domainWebhook.WebhookEndpoint) (*domainWebhook.WebhookDelivery, error)
}

// Service coordinates webhook management, SSRF validation, secret rotations, and audit logging.
type Service struct {
	webhookRepo  domainWebhook.WebhookRepository
	deliveryRepo domainWebhook.DeliveryRepository
	tester       WebhookTester
	authz        identity.Authorizer
	auditRepo    audit.Repository
}

func NewService(
	webhookRepo domainWebhook.WebhookRepository,
	deliveryRepo domainWebhook.DeliveryRepository,
	tester WebhookTester,
	authz identity.Authorizer,
	auditRepo audit.Repository,
) *Service {
	return &Service{
		webhookRepo:  webhookRepo,
		deliveryRepo: deliveryRepo,
		tester:       tester,
		authz:        authz,
		auditRepo:    auditRepo,
	}
}

func (s *Service) CreateEndpoint(ctx context.Context, sub *identity.Subject, input CreateWebhookInput) (*WebhookCreatedResponse, error) {
	if err := s.authz.Authorize(ctx, sub, "webhook:create", nil); err != nil {
		return nil, err
	}

	if input.Name == "" {
		return nil, errors.New("webhook name is required")
	}
	if input.URL == "" {
		return nil, errors.New("webhook URL is required")
	}

	// Validate SSRF
	if err := ssrf.ValidateURL(input.URL); err != nil {
		return nil, fmt.Errorf("%w: %v", domainWebhook.ErrSSRFBlocked, err)
	}

	active := true
	if input.Active != nil {
		active = *input.Active
	}
	eventTypes := input.EventTypes
	if len(eventTypes) == 0 {
		eventTypes = []string{"*"}
	}

	secret := domainWebhook.GenerateSecret()

	ep := &domainWebhook.WebhookEndpoint{
		ID:          uuid.NewString(),
		TenantID:    sub.UserID,
		Name:        input.Name,
		URL:         input.URL,
		Description: input.Description,
		Secret:      secret,
		Active:      active,
		EventTypes:  eventTypes,
	}

	if err := s.webhookRepo.Create(ctx, ep); err != nil {
		return nil, err
	}

	s.logAudit(ctx, sub, "webhook.created", "webhook", ep.ID, map[string]interface{}{
		"name":       ep.Name,
		"url":        ep.URL,
		"eventTypes": ep.EventTypes,
		"active":     ep.Active,
	})

	return &WebhookCreatedResponse{
		Endpoint: ep,
		Secret:   secret,
	}, nil
}

func (s *Service) ListEndpoints(ctx context.Context, sub *identity.Subject, filter domainWebhook.WebhookFilter) ([]*domainWebhook.WebhookEndpoint, int64, error) {
	if err := s.authz.Authorize(ctx, sub, "webhook:read", nil); err != nil {
		return nil, 0, err
	}

	if !sub.HasPermission("*") {
		filter.TenantID = sub.UserID
	}

	return s.webhookRepo.List(ctx, filter)
}

func (s *Service) GetEndpoint(ctx context.Context, sub *identity.Subject, id string) (*domainWebhook.WebhookEndpoint, error) {
	if err := s.authz.Authorize(ctx, sub, "webhook:read", nil); err != nil {
		return nil, err
	}

	ep, err := s.webhookRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if !sub.HasPermission("*") && ep.TenantID != sub.UserID {
		return nil, ErrUnauthorizedTenant
	}

	return ep, nil
}

func (s *Service) UpdateEndpoint(ctx context.Context, sub *identity.Subject, id string, input UpdateWebhookInput) (*domainWebhook.WebhookEndpoint, error) {
	if err := s.authz.Authorize(ctx, sub, "webhook:update", nil); err != nil {
		return nil, err
	}

	ep, err := s.webhookRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !sub.HasPermission("*") && ep.TenantID != sub.UserID {
		return nil, ErrUnauthorizedTenant
	}

	if input.Name != nil && *input.Name != "" {
		ep.Name = *input.Name
	}
	if input.URL != nil && *input.URL != "" {
		if err := ssrf.ValidateURL(*input.URL); err != nil {
			return nil, fmt.Errorf("%w: %v", domainWebhook.ErrSSRFBlocked, err)
		}
		ep.URL = *input.URL
	}
	if input.Description != nil {
		ep.Description = *input.Description
	}
	if input.Active != nil {
		ep.Active = *input.Active
	}
	if input.EventTypes != nil && len(input.EventTypes) > 0 {
		ep.EventTypes = input.EventTypes
	}

	if err := s.webhookRepo.Update(ctx, ep); err != nil {
		return nil, err
	}

	s.logAudit(ctx, sub, "webhook.updated", "webhook", ep.ID, map[string]interface{}{
		"name":       ep.Name,
		"url":        ep.URL,
		"eventTypes": ep.EventTypes,
		"active":     ep.Active,
	})

	return ep, nil
}

func (s *Service) DeleteEndpoint(ctx context.Context, sub *identity.Subject, id string) error {
	if err := s.authz.Authorize(ctx, sub, "webhook:delete", nil); err != nil {
		return err
	}

	ep, err := s.webhookRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if !sub.HasPermission("*") && ep.TenantID != sub.UserID {
		return ErrUnauthorizedTenant
	}

	if err := s.webhookRepo.Delete(ctx, id); err != nil {
		return err
	}

	s.logAudit(ctx, sub, "webhook.deleted", "webhook", id, map[string]interface{}{
		"name": ep.Name,
		"url":  ep.URL,
	})

	return nil
}

func (s *Service) RotateSecret(ctx context.Context, sub *identity.Subject, id string) (string, error) {
	if err := s.authz.Authorize(ctx, sub, "webhook:rotate", nil); err != nil {
		return "", err
	}

	ep, err := s.webhookRepo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	if !sub.HasPermission("*") && ep.TenantID != sub.UserID {
		return "", ErrUnauthorizedTenant
	}

	newSecret := ep.RotateSecret()
	if err := s.webhookRepo.Update(ctx, ep); err != nil {
		return "", err
	}

	// Audit rotation without ever logging the secret itself
	s.logAudit(ctx, sub, "webhook.rotate_secret", "webhook", ep.ID, map[string]interface{}{
		"name": ep.Name,
		"url":  ep.URL,
	})

	return newSecret, nil
}

func (s *Service) TestEndpoint(ctx context.Context, sub *identity.Subject, id string) (*domainWebhook.WebhookDelivery, error) {
	if err := s.authz.Authorize(ctx, sub, "webhook:test", nil); err != nil {
		return nil, err
	}

	ep, err := s.webhookRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !sub.HasPermission("*") && ep.TenantID != sub.UserID {
		return nil, ErrUnauthorizedTenant
	}

	if s.tester == nil {
		return nil, errors.New("webhook tester is unavailable")
	}

	delivery, err := s.tester.TestWebhook(ctx, ep)

	s.logAudit(ctx, sub, "webhook.tested", "webhook", ep.ID, map[string]interface{}{
		"name":   ep.Name,
		"status": delivery.Status,
	})

	return delivery, err
}

func (s *Service) ListDeliveries(ctx context.Context, sub *identity.Subject, filter domainWebhook.DeliveryFilter) ([]*domainWebhook.WebhookDelivery, int64, error) {
	if err := s.authz.Authorize(ctx, sub, "webhook:read", nil); err != nil {
		return nil, 0, err
	}

	if !sub.HasPermission("*") {
		filter.TenantID = sub.UserID
	}

	return s.deliveryRepo.List(ctx, filter)
}

func (s *Service) logAudit(ctx context.Context, sub *identity.Subject, action, resourceType, resourceID string, details map[string]interface{}) {
	if s.auditRepo == nil {
		return
	}
	var actorID *string
	if sub != nil && sub.UserID != "" {
		act := sub.UserID
		actorID = &act
	}
	var resID *string
	if resourceID != "" {
		rID := resourceID
		resID = &rID
	}
	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      actorID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resID,
		Details:      details,
		Severity:     audit.SeverityInfo,
		CreatedAt:    time.Now().UTC(),
	})
}
