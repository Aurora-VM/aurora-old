package billing

import (
	"testing"
	"time"
)

func TestPlan_Validation(t *testing.T) {
	validPlan := &Plan{
		ID:                "plan-pro",
		Name:              "Pro Developer",
		Slug:              "pro-dev",
		Description:       "Dedicated compute tier",
		Currency:          "EUR",
		MonthlyPriceMinor: 2000,
		YearlyPriceMinor:  20000,
		IncludedVCPU:      4,
		IncludedMemoryMB:  8192,
		IncludedStorageMB: 81920,
		MaximumInstances:  10,
		MaximumVCPU:       16,
		MaximumMemoryMB:   32768,
		MaximumStorageMB:  327680,
		Active:            true,
	}

	if err := validPlan.Validate(); err != nil {
		t.Fatalf("expected valid plan, got err: %v", err)
	}

	// Negative price should fail
	invalidPrice := *validPlan
	invalidPrice.MonthlyPriceMinor = -100
	if err := invalidPrice.Validate(); err == nil {
		t.Errorf("expected error on negative price")
	}

	// Invalid slug
	invalidSlug := *validPlan
	invalidSlug.Slug = "INVALID_SLUG!!"
	if err := invalidSlug.Validate(); err == nil {
		t.Errorf("expected error on invalid slug")
	}

	// Invalid currency
	invalidCurr := *validPlan
	invalidCurr.Currency = "EUROPE"
	if err := invalidCurr.Validate(); err == nil {
		t.Errorf("expected error on invalid currency")
	}
}

func TestQuota_Allocation(t *testing.T) {
	q := &Quota{
		TenantID:     "tenant-01",
		Metric:       MetricVCPUHours,
		Limit:        16,
		CurrentUsage: 12,
	}

	if q.Available() != 4 {
		t.Errorf("expected available 4, got %d", q.Available())
	}

	if !q.CanAllocate(4) {
		t.Errorf("expected 4 units to be allocatable")
	}

	if q.CanAllocate(5) {
		t.Errorf("expected 5 units to exceed limit")
	}
}

func TestInvoice_Validation(t *testing.T) {
	inv := &Invoice{
		ID:            "inv-01",
		TenantID:      "tenant-01",
		Currency:      "EUR",
		SubtotalMinor: 2000,
		TaxMinor:      400,
		TotalMinor:    2400,
		Status:        InvoiceStatusOpen,
		PeriodStart:   time.Now().Add(-24 * time.Hour),
		PeriodEnd:     time.Now(),
		DueAt:         time.Now().Add(7 * 24 * time.Hour),
	}

	if err := inv.Validate(); err != nil {
		t.Fatalf("expected valid invoice, got: %v", err)
	}

	// Mismatched subtotal + tax
	inv.TotalMinor = 2500
	if err := inv.Validate(); err == nil {
		t.Errorf("expected error on total sum mismatch")
	}
}
