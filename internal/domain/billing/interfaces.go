package billing

import (
	"context"
	"time"
)

// PlanRepository defines persistence port for product billing plans.
type PlanRepository interface {
	Create(ctx context.Context, plan *Plan) error
	GetByID(ctx context.Context, id string) (*Plan, error)
	GetBySlug(ctx context.Context, slug string) (*Plan, error)
	List(ctx context.Context, activeOnly bool) ([]*Plan, error)
	Update(ctx context.Context, plan *Plan) error
	Delete(ctx context.Context, id string) error
}

// SubscriptionRepository defines persistence port for tenant subscriptions.
type SubscriptionRepository interface {
	Create(ctx context.Context, sub *Subscription) error
	GetByID(ctx context.Context, id string) (*Subscription, error)
	GetByTenantID(ctx context.Context, tenantID string) (*Subscription, error)
	List(ctx context.Context) ([]*Subscription, error)
	Update(ctx context.Context, sub *Subscription) error
	Cancel(ctx context.Context, id string) error
}

// UsageRepository defines persistence port for billable telemetry records.
type UsageRepository interface {
	RecordUsage(ctx context.Context, record *UsageRecord) error
	GetAggregate(ctx context.Context, tenantID string, start, end time.Time) (*UsageAggregate, error)
	ListByTenant(ctx context.Context, tenantID string, limit, offset int) ([]*UsageRecord, error)
}

// QuotaRepository defines persistence port for atomic tenant capacity tracking.
type QuotaRepository interface {
	GetQuota(ctx context.Context, tenantID string, metric MetricType) (*Quota, error)
	ListQuotas(ctx context.Context, tenantID string) (QuotaSet, error)
	SetQuota(ctx context.Context, quota *Quota) error
	ReserveQuota(ctx context.Context, tenantID string, metric MetricType, delta int64, limit int64) error
	ReleaseQuota(ctx context.Context, tenantID string, metric MetricType, delta int64) error
}

// InvoiceRepository defines persistence port for financial invoices and lines.
type InvoiceRepository interface {
	Create(ctx context.Context, invoice *Invoice) error
	GetByID(ctx context.Context, id string) (*Invoice, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*Invoice, error)
	ListByTenant(ctx context.Context, tenantID string, limit, offset int) ([]*Invoice, error)
	ListAll(ctx context.Context, limit, offset int) ([]*Invoice, error)
	UpdateStatus(ctx context.Context, id string, status InvoiceStatus, paidAt *time.Time) error
}

// PaymentResult describes output from payment processing.
type PaymentResult struct {
	PaymentID      string
	IdempotencyKey string
	AmountMinor    int64
	Currency       string
	Status         string // "succeeded", "failed", "pending"
	FailureReason  string
	CreatedAt      time.Time
}

// PaymentProvider defines integration port with upstream payment processors (e.g. Stripe).
type PaymentProvider interface {
	CreateCustomer(ctx context.Context, tenantID, email, name string) (string, error)
	CreatePayment(ctx context.Context, idempotencyKey string, amountMinor int64, currency, description string) (*PaymentResult, error)
	Refund(ctx context.Context, paymentID string, amountMinor int64) error
	GetPaymentStatus(ctx context.Context, paymentID string) (*PaymentResult, error)
}
