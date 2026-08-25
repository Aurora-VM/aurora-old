package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/audit"
	"github.com/aurora-vm/aurora/internal/domain/events"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainNotification "github.com/aurora-vm/aurora/internal/domain/notification"
	"github.com/aurora-vm/aurora/internal/infra/email"
	"github.com/google/uuid"
)

// Service coordinates user notifications, preference filters, and email dispatching.
type Service struct {
	notifRepo domainNotification.NotificationRepository
	prefRepo  domainNotification.PreferenceRepository
	emailProv email.EmailProvider
	authz     identity.Authorizer
	auditRepo audit.Repository
}

func NewService(
	notifRepo domainNotification.NotificationRepository,
	prefRepo domainNotification.PreferenceRepository,
	emailProv email.EmailProvider,
	authz identity.Authorizer,
	auditRepo audit.Repository,
) *Service {
	return &Service{
		notifRepo: notifRepo,
		prefRepo:  prefRepo,
		emailProv: emailProv,
		authz:     authz,
		auditRepo: auditRepo,
	}
}

// HandleEvent processes incoming domain events, creates in-app notifications, and sends email alerts.
func (s *Service) HandleEvent(ctx context.Context, event *events.Event) error {
	// If event has an associated tenant/user, determine notification title & body
	title, body, severity := formatEventNotification(event)

	// 1. In-App Notification
	pref, _ := s.prefRepo.GetPreference(ctx, event.TenantID, string(event.Type))
	if pref == nil || pref.InAppEnabled {
		notif := &domainNotification.Notification{
			ID:           uuid.NewString(),
			TenantID:     event.TenantID,
			UserID:       event.TenantID,
			Type:         string(event.Type),
			Title:        title,
			Body:         body,
			Severity:     severity,
			ResourceType: event.ResourceType,
			ResourceID:   event.ResourceID,
			CreatedAt:    time.Now().UTC(),
		}
		_ = s.notifRepo.Create(ctx, notif)
	}

	// 2. Email Notification
	if s.emailProv != nil && (pref == nil || pref.EmailEnabled) {
		targetEmail := fmt.Sprintf("%s@aurora.local", event.TenantID)
		_ = s.emailProv.SendEventNotification(ctx, targetEmail, event)
	}

	return nil
}

func (s *Service) ListNotifications(ctx context.Context, sub *identity.Subject, filter domainNotification.Filter) ([]*domainNotification.Notification, int64, error) {
	if err := s.authz.Authorize(ctx, sub, "notification:read", nil); err != nil {
		return nil, 0, err
	}

	// Non-superadmins are strictly restricted to their own tenant/user notifications
	if !sub.HasPermission("*") {
		filter.TenantID = sub.UserID
		filter.UserID = sub.UserID
	}

	return s.notifRepo.List(ctx, filter)
}

func (s *Service) MarkAsRead(ctx context.Context, sub *identity.Subject, id string) error {
	if err := s.authz.Authorize(ctx, sub, "notification:manage", nil); err != nil {
		return err
	}
	return s.notifRepo.MarkAsRead(ctx, id, sub.UserID)
}

func (s *Service) MarkAllAsRead(ctx context.Context, sub *identity.Subject) (int64, error) {
	if err := s.authz.Authorize(ctx, sub, "notification:manage", nil); err != nil {
		return 0, err
	}
	return s.notifRepo.MarkAllAsRead(ctx, sub.UserID)
}

func (s *Service) GetUnreadCount(ctx context.Context, sub *identity.Subject) (int64, error) {
	if err := s.authz.Authorize(ctx, sub, "notification:read", nil); err != nil {
		return 0, err
	}
	return s.notifRepo.GetUnreadCount(ctx, sub.UserID)
}

func (s *Service) GetPreferences(ctx context.Context, sub *identity.Subject) ([]*domainNotification.NotificationPreference, error) {
	if err := s.authz.Authorize(ctx, sub, "notification:read", nil); err != nil {
		return nil, err
	}
	return s.prefRepo.GetPreferences(ctx, sub.UserID)
}

func (s *Service) SetPreference(ctx context.Context, sub *identity.Subject, pref *domainNotification.NotificationPreference) error {
	if err := s.authz.Authorize(ctx, sub, "notification:manage", nil); err != nil {
		return err
	}
	pref.UserID = sub.UserID
	err := s.prefRepo.SetPreference(ctx, pref)
	if err == nil {
		s.logAudit(ctx, sub, "notification.preference_update", "user", sub.UserID, map[string]interface{}{
			"eventType": pref.EventType,
			"inApp":     pref.InAppEnabled,
			"email":     pref.EmailEnabled,
			"webhook":   pref.WebhookEnabled,
		})
	}
	return err
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

func formatEventNotification(e *events.Event) (string, string, domainNotification.Severity) {
	switch e.Type {
	case events.EventInstanceCreated:
		return "Instance Provisioned", fmt.Sprintf("Instance %s (%s) was provisioned successfully.", e.ResourceID, e.ResourceType), domainNotification.SeveritySuccess
	case events.EventInstanceDeleted:
		return "Instance Terminated", fmt.Sprintf("Instance %s has been deleted.", e.ResourceID), domainNotification.SeverityInfo
	case events.EventInstanceStarted:
		return "Instance Started", fmt.Sprintf("Instance %s is now running.", e.ResourceID), domainNotification.SeveritySuccess
	case events.EventInstanceStopped:
		return "Instance Stopped", fmt.Sprintf("Instance %s has stopped.", e.ResourceID), domainNotification.SeverityInfo
	case events.EventInstanceError:
		return "Instance Failure", fmt.Sprintf("Instance %s encountered an error during operation.", e.ResourceID), domainNotification.SeverityError
	case events.EventSubscriptionCreated:
		return "Subscription Active", fmt.Sprintf("Subscription to plan %s is now active.", e.ResourceID), domainNotification.SeveritySuccess
	case events.EventInvoiceCreated:
		return "New Invoice Generated", fmt.Sprintf("Invoice %s has been generated for your account.", e.ResourceID), domainNotification.SeverityInfo
	case events.EventQuotaExceeded:
		return "Quota Limit Exceeded", fmt.Sprintf("Resource allocation request exceeded your current subscription quota."), domainNotification.SeverityWarning
	case events.EventMonitoringAlert:
		return "Monitoring Alert Triggered", fmt.Sprintf("High resource threshold exceeded for %s.", e.ResourceID), domainNotification.SeverityWarning
	case events.EventAuditIntegrityFailure:
		return "Security Incident Detected", "Audit ledger hash verification failed. Possible tamper attempt.", domainNotification.SeverityCritical
	default:
		return fmt.Sprintf("Event: %s", string(e.Type)), fmt.Sprintf("Event occurred on %s (%s)", e.ResourceID, e.ResourceType), domainNotification.SeverityInfo
	}
}
