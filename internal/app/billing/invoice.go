package billing

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/billing"
	"github.com/google/uuid"
)

// Unit prices for resource overages (in minor units / EUR cents)
const (
	OveragePriceVCPUHourMinor    int64 = 1   // 1 cent per vCPU-hour overage
	OveragePriceRAMGBHourMinor   int64 = 1   // 1 cent per RAM GB-hour overage
	OveragePriceStorageGBMoMinor int64 = 8   // 8 cents per Storage GB-month overage
	OveragePriceBandwidthGBMinor int64 = 1   // 1 cent per Bandwidth GB overage
	OveragePriceIPv4MonthlyMinor int64 = 200 // €2.00 per extra IPv4 / month
)

// InvoiceEngine creates deterministic financial invoices for subscription periods.
type InvoiceEngine struct {
	planRepo    billing.PlanRepository
	subRepo     billing.SubscriptionRepository
	usageRepo   billing.UsageRepository
	invoiceRepo billing.InvoiceRepository
	paymentProv billing.PaymentProvider
	taxPercent  float64 // e.g. 0.0 or 20.0
}

func NewInvoiceEngine(
	planRepo billing.PlanRepository,
	subRepo billing.SubscriptionRepository,
	usageRepo billing.UsageRepository,
	invoiceRepo billing.InvoiceRepository,
	paymentProv billing.PaymentProvider,
	taxPercent float64,
) *InvoiceEngine {
	return &InvoiceEngine{
		planRepo:    planRepo,
		subRepo:     subRepo,
		usageRepo:   usageRepo,
		invoiceRepo: invoiceRepo,
		paymentProv: paymentProv,
		taxPercent:  taxPercent,
	}
}

// GenerateInvoice compiles base subscription fees and overages into an immutable, persisted invoice.
func (ie *InvoiceEngine) GenerateInvoice(
	ctx context.Context,
	tenantID string,
	periodStart time.Time,
	periodEnd time.Time,
	idempotencyKey string,
) (*billing.Invoice, error) {
	if idempotencyKey != "" {
		existing, err := ie.invoiceRepo.GetByIdempotencyKey(ctx, idempotencyKey)
		if err == nil && existing != nil {
			return existing, nil
		}
	}

	// 1. Load subscription
	sub, err := ie.subRepo.GetByTenantID(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("unable to load tenant subscription: %w", err)
	}

	// 2. Load plan
	plan, err := ie.planRepo.GetByID(ctx, sub.PlanID)
	if err != nil {
		return nil, fmt.Errorf("unable to load plan %s: %w", sub.PlanID, err)
	}

	// 3. Load usage aggregates
	agg, err := ie.usageRepo.GetAggregate(ctx, tenantID, periodStart, periodEnd)
	if err != nil {
		return nil, fmt.Errorf("unable to load usage aggregate: %w", err)
	}

	var lines []*billing.InvoiceLine
	var subtotalMinor int64

	// Base Plan Subscription Line Item
	basePrice := plan.MonthlyPriceMinor
	if sub.BillingCycle == billing.BillingCycleYearly {
		basePrice = plan.YearlyPriceMinor
	}
	subtotalMinor += basePrice

	lines = append(lines, &billing.InvoiceLine{
		ID:             uuid.NewString(),
		Description:    fmt.Sprintf("Subscription Tier: %s (%s)", plan.Name, sub.BillingCycle),
		Quantity:       1.0,
		UnitPriceMinor: basePrice,
		TotalMinor:     basePrice,
	})

	// Nominal hours in month for allowance calculation
	nominalHours := 730.0
	if sub.BillingCycle == billing.BillingCycleYearly {
		nominalHours = 8760.0
	}

	// 4. Calculate Overages
	// a. vCPU-hours
	includedVCPUHours := float64(plan.IncludedVCPU) * nominalHours
	actualVCPUHours := agg.Metrics[billing.MetricVCPUHours]
	if actualVCPUHours > includedVCPUHours {
		overageVCPU := actualVCPUHours - includedVCPUHours
		overageVCPUQty := math.Ceil(overageVCPU)
		cost := int64(overageVCPUQty) * OveragePriceVCPUHourMinor
		subtotalMinor += cost
		lines = append(lines, &billing.InvoiceLine{
			ID:             uuid.NewString(),
			Description:    fmt.Sprintf("Compute Overage: vCPU-hours (Included: %.0f, Used: %.1f)", includedVCPUHours, actualVCPUHours),
			Metric:         billing.MetricVCPUHours,
			Quantity:       overageVCPU,
			UnitPriceMinor: OveragePriceVCPUHourMinor,
			TotalMinor:     cost,
		})
	}

	// b. RAM GB-hours
	includedRAMGBHours := (float64(plan.IncludedMemoryMB) / 1024.0) * nominalHours
	actualRAMGBHours := agg.Metrics[billing.MetricRAMGBHours]
	if actualRAMGBHours > includedRAMGBHours {
		overageRAM := actualRAMGBHours - includedRAMGBHours
		overageRAMQty := math.Ceil(overageRAM)
		cost := int64(overageRAMQty) * OveragePriceRAMGBHourMinor
		subtotalMinor += cost
		lines = append(lines, &billing.InvoiceLine{
			ID:             uuid.NewString(),
			Description:    fmt.Sprintf("Memory Overage: RAM GB-hours (Included: %.0f, Used: %.1f)", includedRAMGBHours, actualRAMGBHours),
			Metric:         billing.MetricRAMGBHours,
			Quantity:       overageRAM,
			UnitPriceMinor: OveragePriceRAMGBHourMinor,
			TotalMinor:     cost,
		})
	}

	// c. Storage GB-months
	includedStorageGBMo := float64(plan.IncludedStorageMB) / 1024.0
	actualStorageGBMo := agg.Metrics[billing.MetricStorageGBMonths]
	if actualStorageGBMo > includedStorageGBMo {
		overageStorage := actualStorageGBMo - includedStorageGBMo
		overageStorageQty := math.Ceil(overageStorage)
		cost := int64(overageStorageQty) * OveragePriceStorageGBMoMinor
		subtotalMinor += cost
		lines = append(lines, &billing.InvoiceLine{
			ID:             uuid.NewString(),
			Description:    fmt.Sprintf("Storage Overage: GB-months (Included: %.0f, Used: %.1f)", includedStorageGBMo, actualStorageGBMo),
			Metric:         billing.MetricStorageGBMonths,
			Quantity:       overageStorage,
			UnitPriceMinor: OveragePriceStorageGBMoMinor,
			TotalMinor:     cost,
		})
	}

	// d. Bandwidth / Network Egress GB
	includedBandwidth := float64(plan.IncludedBandwidthGB)
	actualBandwidth := agg.Metrics[billing.MetricNetworkEgressGB]
	if actualBandwidth > includedBandwidth {
		overageBW := actualBandwidth - includedBandwidth
		overageBWQty := math.Ceil(overageBW)
		cost := int64(overageBWQty) * OveragePriceBandwidthGBMinor
		subtotalMinor += cost
		lines = append(lines, &billing.InvoiceLine{
			ID:             uuid.NewString(),
			Description:    fmt.Sprintf("Bandwidth Overage: Egress GB (Included: %.0f, Used: %.1f)", includedBandwidth, actualBandwidth),
			Metric:         billing.MetricNetworkEgressGB,
			Quantity:       overageBW,
			UnitPriceMinor: OveragePriceBandwidthGBMinor,
			TotalMinor:     cost,
		})
	}

	// 5. Calculate Tax & Total
	var taxMinor int64
	if ie.taxPercent > 0 {
		taxMinor = int64(math.Round(float64(subtotalMinor) * (ie.taxPercent / 100.0)))
	}
	totalMinor := subtotalMinor + taxMinor

	now := time.Now().UTC()
	invoice := &billing.Invoice{
		ID:             uuid.NewString(),
		TenantID:       tenantID,
		SubscriptionID: sub.ID,
		Currency:       plan.Currency,
		SubtotalMinor:  subtotalMinor,
		TaxMinor:       taxMinor,
		TotalMinor:     totalMinor,
		Status:         billing.InvoiceStatusOpen,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		DueAt:          now.Add(14 * 24 * time.Hour), // Net 14 days
		IdempotencyKey: idempotencyKey,
		Lines:          lines,
		CreatedAt:      now,
	}

	if err := invoice.Validate(); err != nil {
		return nil, err
	}

	// 6. Process payment via provider
	if ie.paymentProv != nil && totalMinor > 0 {
		payKey := fmt.Sprintf("pay-%s", invoice.ID)
		payRes, err := ie.paymentProv.CreatePayment(ctx, payKey, totalMinor, plan.Currency, fmt.Sprintf("Invoice %s for %s", invoice.ID, plan.Name))
		if err == nil && payRes != nil && payRes.Status == "succeeded" {
			invoice.Status = billing.InvoiceStatusPaid
			invoice.PaidAt = &now
		}
	} else if totalMinor == 0 {
		invoice.Status = billing.InvoiceStatusPaid
		invoice.PaidAt = &now
	}

	if err := ie.invoiceRepo.Create(ctx, invoice); err != nil {
		return nil, err
	}

	return invoice, nil
}
