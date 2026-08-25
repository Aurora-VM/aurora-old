package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appBilling "github.com/aurora-vm/aurora/internal/app/billing"
	"github.com/aurora-vm/aurora/internal/domain/audit"
	"github.com/aurora-vm/aurora/internal/domain/billing"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	transportHTTP "github.com/aurora-vm/aurora/internal/transport/http"
	"github.com/go-chi/chi/v5"
)

type testAuthorizer struct{}

func (a *testAuthorizer) Authorize(ctx context.Context, sub *identity.Subject, perm string, res *identity.Resource) error {
	if sub == nil {
		return identity.ErrUnauthorized
	}
	if sub.HasPermission("*") || sub.HasPermission(perm) {
		return nil
	}
	return identity.ErrInsufficientPermission
}

type testAuditRepo struct{}

func (a *testAuditRepo) Record(ctx context.Context, log *audit.AuditLog) error {
	return nil
}
func (a *testAuditRepo) ListFiltered(ctx context.Context, filter audit.AuditFilter) ([]*audit.AuditLog, int64, error) {
	return nil, 0, nil
}
func (a *testAuditRepo) GetLatestLog(ctx context.Context) (*audit.AuditLog, error) {
	return nil, nil
}
func (a *testAuditRepo) VerifyChainIntegrity(ctx context.Context, limit int) (bool, int64, error) {
	return true, 0, nil
}

func setupBillingHTTPTest() (chi.Router, *appBilling.Service, *identity.Subject, *identity.Subject) {
	planRepo := memory.NewMemoryPlanRepo()
	subRepo := memory.NewMemorySubscriptionRepo()
	quotaRepo := memory.NewMemoryQuotaRepo()
	usageRepo := memory.NewMemoryUsageRepo()
	invoiceRepo := memory.NewMemoryInvoiceRepo()
	paymentProv := appBilling.NewSimulatedPaymentProvider()
	authz := &testAuthorizer{}
	auditRepo := &testAuditRepo{}

	billingService := appBilling.NewService(
		planRepo,
		subRepo,
		quotaRepo,
		usageRepo,
		invoiceRepo,
		paymentProv,
		authz,
		auditRepo,
	)

	customerSubject := &identity.Subject{
		UserID:      "user-cust-01",
		Username:    "customer1",
		Roles:       []string{"customer"},
		Permissions: []string{"billing:read", "billing:manage"},
	}

	adminSubject := &identity.Subject{
		UserID:      "user-admin-01",
		Username:    "superadmin",
		Roles:       []string{"superadmin"},
		Permissions: []string{"*"},
	}

	r := transportHTTP.NewRouter()
	handler := transportHTTP.NewBillingHandler(billingService, authz)

	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			sub := customerSubject
			if req.Header.Get("X-Test-Role") == "admin" {
				sub = adminSubject
			}
			ctx := context.WithValue(req.Context(), transportHTTP.SubjectContextKey, sub)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}

	handler.RegisterRoutes(r, authMiddleware)

	return r, billingService, customerSubject, adminSubject
}

func TestBillingHandler_Customer_Flow(t *testing.T) {
	r, svc, custSub, _ := setupBillingHTTPTest()

	// 1. List Plans
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/billing/plans", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data struct {
			Plans []*billing.Plan `json:"plans"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Data.Plans) < 3 {
		t.Fatalf("expected at least 3 plans, got %d", len(resp.Data.Plans))
	}
	targetPlan := resp.Data.Plans[1]

	// 2. Subscribe to Plan
	subBody, _ := json.Marshal(appBilling.SubscribeRequest{
		PlanID:       targetPlan.ID,
		BillingCycle: billing.BillingCycleMonthly,
	})
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/billing/subscription", bytes.NewReader(subBody))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// 3. Get Current Subscription & Plan
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/billing/subscription", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	// 4. Get Quotas
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/billing/quotas", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	// 5. Generate Usage and Invoices
	start := time.Now().Add(-30 * 24 * time.Hour)
	end := time.Now()
	_ = svc.MeterEngine().RecordInstanceUsage(context.Background(), custSub.UserID, "inst-http-01", 2, 2048*1024*1024, 20*1024*1024*1024, start, end)
	_, _ = svc.InvoiceEngine().GenerateInvoice(context.Background(), custSub.UserID, start, end, "inv-test-http-01")

	// 6. List Invoices
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/billing/invoices", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var invResp struct {
		Data struct {
			Invoices []*billing.Invoice `json:"invoices"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &invResp)
	if len(invResp.Data.Invoices) == 0 {
		t.Fatalf("expected at least 1 invoice")
	}

	// 7. Get Single Invoice Detail
	invID := invResp.Data.Invoices[0].ID
	req, _ = http.NewRequest(http.MethodGet, "/api/v1/billing/invoices/"+invID, nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 for invoice detail, got %d", rr.Code)
	}
}

func TestBillingHandler_Admin_RBAC(t *testing.T) {
	r, _, _, _ := setupBillingHTTPTest()

	// Customer attempting to call admin plan creation should be forbidden (403)
	planBody, _ := json.Marshal(appBilling.CreatePlanRequest{
		Name:              "Super Plan",
		Slug:              "super-plan",
		Description:       "Super",
		Currency:          "EUR",
		MonthlyPriceMinor: 9900,
	})
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/admin/billing/plans", bytes.NewReader(planBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 Forbidden for customer, got %d: %s", rr.Code, rr.Body.String())
	}

	// Admin calling admin plan creation succeeds (201)
	req, _ = http.NewRequest(http.MethodPost, "/api/v1/admin/billing/plans", bytes.NewReader(planBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Role", "admin")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created for admin, got %d: %s", rr.Code, rr.Body.String())
	}
}
