package billing

import (
	"fmt"
	"time"
)

type InvoiceStatus string

const (
	InvoiceStatusDraft         InvoiceStatus = "draft"
	InvoiceStatusOpen          InvoiceStatus = "open"
	InvoiceStatusPaid          InvoiceStatus = "paid"
	InvoiceStatusVoid          InvoiceStatus = "void"
	InvoiceStatusUncollectible InvoiceStatus = "uncollectible"
)

// InvoiceLine represents an individual line item on a billing invoice.
type InvoiceLine struct {
	ID             string     `json:"id"`
	InvoiceID      string     `json:"invoiceId"`
	Description    string     `json:"description"`
	Metric         MetricType `json:"metric,omitempty"`
	Quantity       float64    `json:"quantity"`
	UnitPriceMinor int64      `json:"unitPriceMinor"`
	TotalMinor     int64      `json:"totalMinor"`
}

// Invoice represents a financial statement for a subscription and periodic usage.
type Invoice struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenantId"`
	SubscriptionID string         `json:"subscriptionId,omitempty"`
	Currency       string         `json:"currency"`
	SubtotalMinor  int64          `json:"subtotalMinor"`
	TaxMinor       int64          `json:"taxMinor"`
	TotalMinor     int64          `json:"totalMinor"`
	Status         InvoiceStatus  `json:"status"`
	PeriodStart    time.Time      `json:"periodStart"`
	PeriodEnd      time.Time      `json:"periodEnd"`
	DueAt          time.Time      `json:"dueAt"`
	PaidAt         *time.Time     `json:"paidAt,omitempty"`
	IdempotencyKey string         `json:"idempotencyKey,omitempty"`
	Lines          []*InvoiceLine `json:"lines,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
}

func (inv *Invoice) Validate() error {
	if inv.TenantID == "" {
		return fmt.Errorf("%w: tenant ID is required", ErrInvalidInvoiceState)
	}
	if inv.Currency == "" {
		return fmt.Errorf("%w: currency is required", ErrInvalidInvoiceState)
	}
	if inv.SubtotalMinor < 0 || inv.TaxMinor < 0 || inv.TotalMinor < 0 {
		return fmt.Errorf("%w: amounts cannot be negative", ErrInvalidInvoiceState)
	}
	if inv.SubtotalMinor+inv.TaxMinor != inv.TotalMinor {
		return fmt.Errorf("%w: subtotal + tax must equal total", ErrInvalidInvoiceState)
	}
	return nil
}
