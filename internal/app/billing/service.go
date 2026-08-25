package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/audit"
	"github.com/aurora-vm/aurora/internal/domain/billing"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/google/uuid"
)

type CreatePlanRequest struct {
	Name                string          `json:"name"`
	Slug                string          `json:"slug"`
	Description         string          `json:"description"`
	Currency            string          `json:"currency"`
	MonthlyPriceMinor   int64           `json:"monthlyPriceMinor"`
	YearlyPriceMinor    int64           `json:"yearlyPriceMinor"`
	IncludedVCPU        int             `json:"includedVcpu"`
	IncludedMemoryMB    int64           `json:"includedMemoryMb"`
	IncludedStorageMB   int64           `json:"includedStorageMb"`
	IncludedIPv4        int             `json:"includedIpv4"`
	IncludedSnapshots   int             `json:"includedSnapshots"`
	IncludedBackups     int             `json:"includedBackups"`
	IncludedBandwidthGB int64           `json:"includedBandwidthGb"`
	MaximumInstances    int             `json:"maximumInstances"`
	MaximumVCPU         int             `json:"maximumVcpu"`
	MaximumMemoryMB     int64           `json:"maximumMemoryMb"`
	MaximumStorageMB    int64           `json:"maximumStorageMb"`
	Features            map[string]bool `json:"features"`
}

type UpdatePlanRequest struct {
	Name                string          `json:"name"`
	Slug                string          `json:"slug"`
	Description         string          `json:"description"`
	Currency            string          `json:"currency"`
	MonthlyPriceMinor   int64           `json:"monthlyPriceMinor"`
	YearlyPriceMinor    int64           `json:"yearlyPriceMinor"`
	IncludedVCPU        int             `json:"includedVcpu"`
	IncludedMemoryMB    int64           `json:"includedMemoryMb"`
	IncludedStorageMB   int64           `json:"includedStorageMb"`
	IncludedIPv4        int             `json:"includedIpv4"`
	IncludedSnapshots   int             `json:"includedSnapshots"`
	IncludedBackups     int             `json:"includedBackups"`
	IncludedBandwidthGB int64           `json:"includedBandwidthGb"`
	MaximumInstances    int             `json:"maximumInstances"`
	MaximumVCPU         int             `json:"maximumVcpu"`
	MaximumMemoryMB     int64           `json:"maximumMemoryMb"`
	MaximumStorageMB    int64           `json:"maximumStorageMb"`
	Features            map[string]bool `json:"features"`
	Active              bool            `json:"active"`
}

type SubscribeRequest struct {
	PlanID       string               `json:"planId"`
	BillingCycle billing.BillingCycle `json:"billingCycle"` // "monthly" or "yearly"
}

type ChangePlanRequest struct {
	NewPlanID    string               `json:"newPlanId"`
	BillingCycle billing.BillingCycle `json:"billingCycle,omitempty"`
}

// Service coordinates plans, subscriptions, quotas, metering, invoices, and audit trails.
type Service struct {
	planRepo    billing.PlanRepository
	subRepo     billing.SubscriptionRepository
	quotaRepo   billing.QuotaRepository
	usageRepo   billing.UsageRepository
	invoiceRepo billing.InvoiceRepository
	paymentProv billing.PaymentProvider
	quotaEngine *QuotaEngine
	meterEngine *UsageMeteringEngine
	invEngine   *InvoiceEngine
	authorizer  identity.Authorizer
	auditRepo   audit.Repository
}

func NewService(
	planRepo billing.PlanRepository,
	subRepo billing.SubscriptionRepository,
	quotaRepo billing.QuotaRepository,
	usageRepo billing.UsageRepository,
	invoiceRepo billing.InvoiceRepository,
	paymentProv billing.PaymentProvider,
	authorizer identity.Authorizer,
	auditRepo audit.Repository,
) *Service {
	quotaEngine := NewQuotaEngine(quotaRepo, planRepo, subRepo)
	meterEngine := NewUsageMeteringEngine(usageRepo)
	invEngine := NewInvoiceEngine(planRepo, subRepo, usageRepo, invoiceRepo, paymentProv, 0.0)

	return &Service{
		planRepo:    planRepo,
		subRepo:     subRepo,
		quotaRepo:   quotaRepo,
		usageRepo:   usageRepo,
		invoiceRepo: invoiceRepo,
		paymentProv: paymentProv,
		quotaEngine: quotaEngine,
		meterEngine: meterEngine,
		invEngine:   invEngine,
		authorizer:  authorizer,
		auditRepo:   auditRepo,
	}
}

func (s *Service) QuotaEngine() *QuotaEngine {
	return s.quotaEngine
}

func (s *Service) MeterEngine() *UsageMeteringEngine {
	return s.meterEngine
}

func (s *Service) InvoiceEngine() *InvoiceEngine {
	return s.invEngine
}

// --- Plan Management (Admin) ---

func (s *Service) CreatePlan(ctx context.Context, sub *identity.Subject, req CreatePlanRequest) (*billing.Plan, error) {
	if err := s.authorizer.Authorize(ctx, sub, "billing:plans", nil); err != nil {
		return nil, err
	}

	plan := &billing.Plan{
		ID:                  uuid.NewString(),
		Name:                req.Name,
		Slug:                req.Slug,
		Description:         req.Description,
		Currency:            req.Currency,
		MonthlyPriceMinor:   req.MonthlyPriceMinor,
		YearlyPriceMinor:    req.YearlyPriceMinor,
		IncludedVCPU:        req.IncludedVCPU,
		IncludedMemoryMB:    req.IncludedMemoryMB,
		IncludedStorageMB:   req.IncludedStorageMB,
		IncludedIPv4:        req.IncludedIPv4,
		IncludedSnapshots:   req.IncludedSnapshots,
		IncludedBackups:     req.IncludedBackups,
		IncludedBandwidthGB: req.IncludedBandwidthGB,
		MaximumInstances:    req.MaximumInstances,
		MaximumVCPU:         req.MaximumVCPU,
		MaximumMemoryMB:     req.MaximumMemoryMB,
		MaximumStorageMB:    req.MaximumStorageMB,
		Features:            req.Features,
		Active:              true,
	}

	if err := plan.Validate(); err != nil {
		return nil, err
	}

	if err := s.planRepo.Create(ctx, plan); err != nil {
		return nil, err
	}

	s.logAudit(ctx, sub, "billing:plan:create", "plan", plan.ID, map[string]interface{}{
		"name": plan.Name,
		"slug": plan.Slug,
	})

	return plan, nil
}

func (s *Service) UpdatePlan(ctx context.Context, sub *identity.Subject, id string, req UpdatePlanRequest) (*billing.Plan, error) {
	if err := s.authorizer.Authorize(ctx, sub, "billing:plans", nil); err != nil {
		return nil, err
	}

	existing, err := s.planRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	existing.Name = req.Name
	existing.Slug = req.Slug
	existing.Description = req.Description
	existing.Currency = req.Currency
	existing.MonthlyPriceMinor = req.MonthlyPriceMinor
	existing.YearlyPriceMinor = req.YearlyPriceMinor
	existing.IncludedVCPU = req.IncludedVCPU
	existing.IncludedMemoryMB = req.IncludedMemoryMB
	existing.IncludedStorageMB = req.IncludedStorageMB
	existing.IncludedIPv4 = req.IncludedIPv4
	existing.IncludedSnapshots = req.IncludedSnapshots
	existing.IncludedBackups = req.IncludedBackups
	existing.IncludedBandwidthGB = req.IncludedBandwidthGB
	existing.MaximumInstances = req.MaximumInstances
	existing.MaximumVCPU = req.MaximumVCPU
	existing.MaximumMemoryMB = req.MaximumMemoryMB
	existing.MaximumStorageMB = req.MaximumStorageMB
	existing.Features = req.Features
	existing.Active = req.Active

	if err := existing.Validate(); err != nil {
		return nil, err
	}

	if err := s.planRepo.Update(ctx, existing); err != nil {
		return nil, err
	}

	s.logAudit(ctx, sub, "billing:plan:update", "plan", existing.ID, map[string]interface{}{
		"slug":   existing.Slug,
		"active": existing.Active,
	})

	return existing, nil
}

func (s *Service) DeactivatePlan(ctx context.Context, sub *identity.Subject, id string) error {
	if err := s.authorizer.Authorize(ctx, sub, "billing:plans", nil); err != nil {
		return err
	}

	existing, err := s.planRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	existing.Active = false
	if err := s.planRepo.Update(ctx, existing); err != nil {
		return err
	}

	s.logAudit(ctx, sub, "billing:plan:deactivate", "plan", id, nil)
	return nil
}

func (s *Service) ListPlans(ctx context.Context, sub *identity.Subject, activeOnly bool) ([]*billing.Plan, error) {
	// Customers can list active plans freely
	return s.planRepo.List(ctx, activeOnly)
}

func (s *Service) GetPlan(ctx context.Context, sub *identity.Subject, id string) (*billing.Plan, error) {
	return s.planRepo.GetByID(ctx, id)
}

// --- Subscription Management (Customer & Admin) ---

func (s *Service) SubscribeTenant(ctx context.Context, sub *identity.Subject, req SubscribeRequest) (*billing.Subscription, error) {
	if err := s.authorizer.Authorize(ctx, sub, "billing:manage", nil); err != nil {
		return nil, err
	}

	tenantID := sub.UserID
	plan, err := s.planRepo.GetByID(ctx, req.PlanID)
	if err != nil {
		return nil, err
	}
	if !plan.Active {
		return nil, billing.ErrPlanInactive
	}

	existing, err := s.subRepo.GetByTenantID(ctx, tenantID)
	if err == nil && existing != nil && existing.Status == billing.SubscriptionStatusActive {
		return nil, billing.ErrSubscriptionAlreadyExists
	}

	now := time.Now().UTC()
	periodEnd := now.AddDate(0, 1, 0)
	if req.BillingCycle == billing.BillingCycleYearly {
		periodEnd = now.AddDate(1, 0, 0)
	}

	subscription := &billing.Subscription{
		ID:                 uuid.NewString(),
		TenantID:           tenantID,
		PlanID:             plan.ID,
		Status:             billing.SubscriptionStatusActive,
		BillingCycle:       req.BillingCycle,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   periodEnd,
		CancelAtPeriodEnd:  false,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := subscription.Validate(); err != nil {
		return nil, err
	}

	if err := s.subRepo.Create(ctx, subscription); err != nil {
		return nil, err
	}

	s.logAudit(ctx, sub, "billing:subscription:create", "subscription", subscription.ID, map[string]interface{}{
		"planId":       plan.ID,
		"planName":     plan.Name,
		"billingCycle": subscription.BillingCycle,
	})

	return subscription, nil
}

func (s *Service) ChangePlan(ctx context.Context, sub *identity.Subject, req ChangePlanRequest) (*billing.Subscription, error) {
	if err := s.authorizer.Authorize(ctx, sub, "billing:manage", nil); err != nil {
		return nil, err
	}

	tenantID := sub.UserID
	subscription, err := s.subRepo.GetByTenantID(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	newPlan, err := s.planRepo.GetByID(ctx, req.NewPlanID)
	if err != nil {
		return nil, err
	}
	if !newPlan.Active {
		return nil, billing.ErrPlanInactive
	}

	oldPlanID := subscription.PlanID
	subscription.PlanID = newPlan.ID
	if req.BillingCycle != "" {
		subscription.BillingCycle = req.BillingCycle
	}
	subscription.UpdatedAt = time.Now().UTC()

	if err := s.subRepo.Update(ctx, subscription); err != nil {
		return nil, err
	}

	s.logAudit(ctx, sub, "billing:subscription:change", "subscription", subscription.ID, map[string]interface{}{
		"oldPlanId": oldPlanID,
		"newPlanId": newPlan.ID,
		"planName":  newPlan.Name,
	})

	return subscription, nil
}

func (s *Service) CancelSubscription(ctx context.Context, sub *identity.Subject) error {
	if err := s.authorizer.Authorize(ctx, sub, "billing:manage", nil); err != nil {
		return err
	}

	tenantID := sub.UserID
	subscription, err := s.subRepo.GetByTenantID(ctx, tenantID)
	if err != nil {
		return err
	}

	subscription.CancelAtPeriodEnd = true
	subscription.Status = billing.SubscriptionStatusCanceled
	subscription.UpdatedAt = time.Now().UTC()

	if err := s.subRepo.Update(ctx, subscription); err != nil {
		return err
	}

	s.logAudit(ctx, sub, "billing:subscription:cancel", "subscription", subscription.ID, nil)
	return nil
}

func (s *Service) GetSubscription(ctx context.Context, sub *identity.Subject) (*billing.Subscription, *billing.Plan, error) {
	if err := s.authorizer.Authorize(ctx, sub, "billing:read", nil); err != nil {
		return nil, nil, err
	}

	subscription, err := s.subRepo.GetByTenantID(ctx, sub.UserID)
	if err != nil {
		// Return default fallback starter plan
		plans, listErr := s.planRepo.List(ctx, true)
		if listErr == nil && len(plans) > 0 {
			return nil, plans[0], nil
		}
		return nil, nil, billing.ErrSubscriptionNotFound
	}

	plan, err := s.planRepo.GetByID(ctx, subscription.PlanID)
	if err != nil {
		return nil, nil, err
	}
	return subscription, plan, nil
}

func (s *Service) GetQuotas(ctx context.Context, sub *identity.Subject) (billing.QuotaSet, *billing.Plan, error) {
	if err := s.authorizer.Authorize(ctx, sub, "billing:read", nil); err != nil {
		return nil, nil, err
	}
	return s.quotaEngine.GetTenantQuotas(ctx, sub.UserID)
}

func (s *Service) GetUsage(ctx context.Context, sub *identity.Subject, start, end time.Time) (*billing.UsageAggregate, error) {
	if err := s.authorizer.Authorize(ctx, sub, "billing:read", nil); err != nil {
		return nil, err
	}
	if start.IsZero() {
		start = time.Now().AddDate(0, -1, 0)
	}
	if end.IsZero() {
		end = time.Now()
	}
	return s.meterEngine.GetAggregateUsage(ctx, sub.UserID, start, end)
}

// --- Invoice Management (Customer & Admin) ---

func (s *Service) ListInvoices(ctx context.Context, sub *identity.Subject, limit, offset int) ([]*billing.Invoice, error) {
	if err := s.authorizer.Authorize(ctx, sub, "billing:read", nil); err != nil {
		return nil, err
	}
	return s.invoiceRepo.ListByTenant(ctx, sub.UserID, limit, offset)
}

func (s *Service) GetInvoice(ctx context.Context, sub *identity.Subject, id string) (*billing.Invoice, error) {
	if err := s.authorizer.Authorize(ctx, sub, "billing:read", nil); err != nil {
		return nil, err
	}

	inv, err := s.invoiceRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Strict tenant isolation: customer can only view own invoices
	if !sub.HasPermission("*") && !sub.HasRole("admin") && !sub.HasRole("superadmin") {
		if inv.TenantID != sub.UserID {
			return nil, billing.ErrInvoiceNotFound
		}
	}
	return inv, nil
}

func (s *Service) ListAllSubscriptions(ctx context.Context, sub *identity.Subject) ([]*billing.Subscription, error) {
	if err := s.authorizer.Authorize(ctx, sub, "billing:admin", nil); err != nil {
		return nil, err
	}
	return s.subRepo.List(ctx)
}

func (s *Service) ListAllInvoices(ctx context.Context, sub *identity.Subject, limit, offset int) ([]*billing.Invoice, error) {
	if err := s.authorizer.Authorize(ctx, sub, "billing:admin", nil); err != nil {
		return nil, err
	}
	return s.invoiceRepo.ListAll(ctx, limit, offset)
}

func (s *Service) VoidInvoice(ctx context.Context, sub *identity.Subject, id string) error {
	if err := s.authorizer.Authorize(ctx, sub, "billing:admin", nil); err != nil {
		return err
	}

	if err := s.invoiceRepo.UpdateStatus(ctx, id, billing.InvoiceStatusVoid, nil); err != nil {
		return err
	}

	s.logAudit(ctx, sub, "billing:invoice:void", "invoice", id, nil)
	return nil
}

func (s *Service) RegenerateInvoice(
	ctx context.Context,
	sub *identity.Subject,
	tenantID string,
	start time.Time,
	end time.Time,
) (*billing.Invoice, error) {
	if err := s.authorizer.Authorize(ctx, sub, "billing:admin", nil); err != nil {
		return nil, err
	}

	idemKey := fmt.Sprintf("regen-%s-%d-%d", tenantID, start.Unix(), end.Unix())
	inv, err := s.invEngine.GenerateInvoice(ctx, tenantID, start, end, idemKey)
	if err != nil {
		return nil, err
	}

	s.logAudit(ctx, sub, "billing:invoice:create", "invoice", inv.ID, map[string]interface{}{
		"tenantId":   tenantID,
		"totalMinor": inv.TotalMinor,
		"status":     inv.Status,
	})

	return inv, nil
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
