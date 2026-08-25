package billing_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appBilling "github.com/aurora-vm/aurora/internal/app/billing"
	"github.com/aurora-vm/aurora/internal/domain/audit"
	"github.com/aurora-vm/aurora/internal/domain/billing"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/aurora-vm/aurora/internal/infra/memory"
)

type mockAuthorizer struct{}

func (m *mockAuthorizer) Authorize(ctx context.Context, sub *identity.Subject, perm string, res *identity.Resource) error {
	return nil
}

func setupBillingTestService() (*appBilling.Service, *memory.MemoryPlanRepo, *memory.MemoryUsageRepo, *memory.MemoryInvoiceRepo) {
	planRepo := memory.NewMemoryPlanRepo()
	subRepo := memory.NewMemorySubscriptionRepo()
	quotaRepo := memory.NewMemoryQuotaRepo()
	usageRepo := memory.NewMemoryUsageRepo()
	invoiceRepo := memory.NewMemoryInvoiceRepo()
	paymentProv := appBilling.NewSimulatedPaymentProvider()
	authorizer := &mockAuthorizer{}
	auditRepo := &mockAuditRepo{}

	svc := appBilling.NewService(
		planRepo,
		subRepo,
		quotaRepo,
		usageRepo,
		invoiceRepo,
		paymentProv,
		authorizer,
		auditRepo,
	)

	return svc, planRepo, usageRepo, invoiceRepo
}

type mockAuditRepo struct{}

func (m *mockAuditRepo) Record(ctx context.Context, log *audit.AuditLog) error {
	return nil
}
func (m *mockAuditRepo) ListFiltered(ctx context.Context, filter audit.AuditFilter) ([]*audit.AuditLog, int64, error) {
	return nil, 0, nil
}
func (m *mockAuditRepo) GetLatestLog(ctx context.Context) (*audit.AuditLog, error) {
	return nil, nil
}
func (m *mockAuditRepo) VerifyChainIntegrity(ctx context.Context, limit int) (bool, int64, error) {
	return true, 0, nil
}

func TestBillingService_PlanAndSubscriptionLifecycle(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := setupBillingTestService()
	subAdmin := &identity.Subject{UserID: "admin-01", Roles: []string{"superadmin"}, Permissions: []string{"*"}}
	subCustomer := &identity.Subject{UserID: "tenant-01", Roles: []string{"customer"}, Permissions: []string{"billing:read", "billing:manage"}}

	// 1. Admin creates custom plan
	customPlan, err := svc.CreatePlan(ctx, subAdmin, appBilling.CreatePlanRequest{
		Name:              "Custom AI Cluster",
		Slug:              "custom-ai",
		Description:       "GPU accelerated nodes",
		Currency:          "EUR",
		MonthlyPriceMinor: 15000,
		YearlyPriceMinor:  150000,
		IncludedVCPU:      32,
		IncludedMemoryMB:  65536,
		IncludedStorageMB: 655360,
		MaximumInstances:  10,
		MaximumVCPU:       64,
		MaximumMemoryMB:   131072,
		MaximumStorageMB:  1310720,
		Features:          map[string]bool{"gpu": true},
	})
	if err != nil {
		t.Fatalf("failed to create plan: %v", err)
	}
	if customPlan.Slug != "custom-ai" {
		t.Errorf("expected slug custom-ai, got %s", customPlan.Slug)
	}

	// 2. List default and custom plans
	plans, err := svc.ListPlans(ctx, subCustomer, true)
	if err != nil {
		t.Fatalf("failed to list plans: %v", err)
	}
	if len(plans) < 4 {
		t.Fatalf("expected at least 4 plans, got %d", len(plans))
	}

	// 2. Subscribe tenant to Pro plan
	proPlan, err := plans[1], error(nil)
	for _, p := range plans {
		if p.Slug == "pro" {
			proPlan = p
			break
		}
	}

	subscription, err := svc.SubscribeTenant(ctx, subCustomer, appBilling.SubscribeRequest{
		PlanID:       proPlan.ID,
		BillingCycle: billing.BillingCycleMonthly,
	})
	if err != nil {
		t.Fatalf("failed to subscribe tenant: %v", err)
	}
	if subscription.Status != billing.SubscriptionStatusActive {
		t.Errorf("expected active subscription, got %s", subscription.Status)
	}

	// 3. Prevent duplicate active subscriptions
	_, err = svc.SubscribeTenant(ctx, subCustomer, appBilling.SubscribeRequest{
		PlanID:       proPlan.ID,
		BillingCycle: billing.BillingCycleMonthly,
	})
	if err == nil {
		t.Errorf("expected error on duplicate active subscription")
	}

	// 4. Change plan to Enterprise
	entPlan := plans[2]
	for _, p := range plans {
		if p.Slug == "enterprise" {
			entPlan = p
			break
		}
	}

	updatedSub, err := svc.ChangePlan(ctx, subCustomer, appBilling.ChangePlanRequest{
		NewPlanID: entPlan.ID,
	})
	if err != nil {
		t.Fatalf("failed to change plan: %v", err)
	}
	if updatedSub.PlanID != entPlan.ID {
		t.Errorf("expected subscription plan to be %s, got %s", entPlan.ID, updatedSub.PlanID)
	}

	// 5. Cancel subscription
	if err := svc.CancelSubscription(ctx, subCustomer); err != nil {
		t.Fatalf("failed to cancel subscription: %v", err)
	}

	curSub, _, err := svc.GetSubscription(ctx, subCustomer)
	if err != nil {
		t.Fatalf("failed to get subscription: %v", err)
	}
	if curSub.Status != billing.SubscriptionStatusCanceled {
		t.Errorf("expected canceled status, got %s", curSub.Status)
	}
}

func TestBillingService_ConcurrentQuotaReservation(t *testing.T) {
	ctx := context.Background()
	svc, planRepo, _, _ := setupBillingTestService()
	quotaEngine := svc.QuotaEngine()
	tenantID := "tenant-concurrent-01"

	// Explicitly subscribe tenant to Pro plan (MaximumVCPU: 16)
	proPlan, err := planRepo.GetBySlug(ctx, "pro")
	if err != nil {
		t.Fatalf("failed to find pro plan: %v", err)
	}
	subSubject := &identity.Subject{UserID: tenantID, Roles: []string{"customer"}, Permissions: []string{"billing:manage"}}
	_, err = svc.SubscribeTenant(ctx, subSubject, appBilling.SubscribeRequest{
		PlanID:       proPlan.ID,
		BillingCycle: billing.BillingCycleMonthly,
	})
	if err != nil {
		t.Fatalf("failed to subscribe tenant to pro: %v", err)
	}

	// Pro plan has max 16 vCPU limit
	// Launch 20 concurrent goroutines, each trying to reserve 4 vCPU
	// Exactly 4 should succeed (4 * 4 = 16 vCPU), and 16 must be rejected.
	var successfulReservations int64
	var failedReservations int64
	var wg sync.WaitGroup

	numGoroutines := 20
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			err := quotaEngine.ReserveQuota(ctx, tenantID, billing.ResourceSpec{
				Instances: 1,
				VCPU:      4,
				MemoryMB:  4096,
				StorageMB: 40960,
			})
			if err == nil {
				atomic.AddInt64(&successfulReservations, 1)
			} else {
				atomic.AddInt64(&failedReservations, 1)
			}
		}(i)
	}

	wg.Wait()

	if successfulReservations > 4 {
		t.Fatalf("quota violated: expected at most 4 successful reservations for 16 vCPU cap, got %d", successfulReservations)
	}
	if successfulReservations+failedReservations != int64(numGoroutines) {
		t.Fatalf("expected total 20 attempts, got %d", successfulReservations+failedReservations)
	}

	// Now release 1 reservation (4 vCPU)
	err = quotaEngine.ReleaseQuota(ctx, tenantID, billing.ResourceSpec{
		Instances: 1,
		VCPU:      4,
		MemoryMB:  4096,
		StorageMB: 40960,
	})
	if err != nil {
		t.Fatalf("failed to release quota: %v", err)
	}

	// Now 1 more reservation of 4 vCPU should succeed
	err = quotaEngine.ReserveQuota(ctx, tenantID, billing.ResourceSpec{
		Instances: 1,
		VCPU:      4,
		MemoryMB:  4096,
		StorageMB: 40960,
	})
	if err != nil {
		t.Fatalf("expected reservation to succeed after release, got: %v", err)
	}
}

func TestBillingService_UsageMeteringAndDeterministicInvoice(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := setupBillingTestService()
	subCustomer := &identity.Subject{UserID: "tenant-meter-01", Roles: []string{"customer"}, Permissions: []string{"billing:read", "billing:manage"}}

	// 1. Subscribe to Starter Plan (€5/month, included: 1 vCPU, 1024MB RAM, 10GB storage)
	plans, _ := svc.ListPlans(ctx, subCustomer, true)
	var starterPlan *billing.Plan
	for _, p := range plans {
		if p.Slug == "starter" {
			starterPlan = p
			break
		}
	}
	if starterPlan == nil {
		t.Fatalf("starter plan not found")
	}
	_, err := svc.SubscribeTenant(ctx, subCustomer, appBilling.SubscribeRequest{
		PlanID:       starterPlan.ID,
		BillingCycle: billing.BillingCycleMonthly,
	})
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	// 2. Record 1000 hours of 4 vCPU, 8GB RAM, 100GB storage (generating overages)
	meterEngine := svc.MeterEngine()
	start := time.Now().Add(-30 * 24 * time.Hour)
	end := time.Now()

	err = meterEngine.RecordInstanceUsage(
		ctx,
		subCustomer.UserID,
		"inst-01",
		4,
		8*1024*1024*1024,
		100*1024*1024*1024,
		start,
		end,
	)
	if err != nil {
		t.Fatalf("failed to record usage: %v", err)
	}

	// 3. Test Idempotent recording (duplicate event must not double charge)
	err = meterEngine.RecordInstanceUsage(
		ctx,
		subCustomer.UserID,
		"inst-01",
		4,
		8*1024*1024*1024,
		100*1024*1024*1024,
		start,
		end,
	)
	if err != nil {
		t.Fatalf("failed on idempotent usage ingestion: %v", err)
	}

	// 4. Generate Invoice
	invEngine := svc.InvoiceEngine()
	idemKey := "inv-2026-08-tenant-meter-01"
	invoice, err := invEngine.GenerateInvoice(ctx, subCustomer.UserID, start, end, idemKey)
	if err != nil {
		t.Fatalf("failed to generate invoice: %v", err)
	}

	if invoice.SubtotalMinor <= starterPlan.MonthlyPriceMinor {
		t.Errorf("expected overage to increase subtotal above base price %d, got %d",
			starterPlan.MonthlyPriceMinor, invoice.SubtotalMinor)
	}

	if len(invoice.Lines) < 2 {
		t.Errorf("expected multiple line items for base plan and overages, got %d", len(invoice.Lines))
	}

	// Replaying invoice generation with same idempotency key returns exact invoice
	replayInv, err := invEngine.GenerateInvoice(ctx, subCustomer.UserID, start, end, idemKey)
	if err != nil {
		t.Fatalf("failed on invoice replay: %v", err)
	}
	if replayInv.ID != invoice.ID {
		t.Errorf("expected idempotent invoice ID %s, got %s", invoice.ID, replayInv.ID)
	}
}
