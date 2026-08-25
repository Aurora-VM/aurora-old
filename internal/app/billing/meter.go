package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/billing"
)

// UsageMeteringEngine aggregates and records billable resource consumption.
type UsageMeteringEngine struct {
	usageRepo billing.UsageRepository
}

func NewUsageMeteringEngine(usageRepo billing.UsageRepository) *UsageMeteringEngine {
	return &UsageMeteringEngine{
		usageRepo: usageRepo,
	}
}

// RecordInstanceUsage calculates and records compute, memory, and disk usage for an instance over a duration.
func (m *UsageMeteringEngine) RecordInstanceUsage(
	ctx context.Context,
	tenantID string,
	instanceID string,
	vcpu int,
	memoryBytes int64,
	storageBytes int64,
	start time.Time,
	end time.Time,
) error {
	durationHours := end.Sub(start).Hours()
	if durationHours <= 0 {
		return nil
	}

	bucketKey := fmt.Sprintf("inst-%s-%d-%d", instanceID, start.Unix(), end.Unix())

	// 1. vCPU-hours
	vcpuHours := float64(vcpu) * durationHours
	err := m.usageRepo.RecordUsage(ctx, &billing.UsageRecord{
		TenantID:       tenantID,
		ResourceType:   "instance",
		ResourceID:     instanceID,
		Metric:         billing.MetricVCPUHours,
		Quantity:       vcpuHours,
		Unit:           "vCPU-hours",
		PeriodStart:    start,
		PeriodEnd:      end,
		IdempotencyKey: fmt.Sprintf("%s-vcpu", bucketKey),
		Metadata:       map[string]interface{}{"vcpu": vcpu, "hours": durationHours},
	})
	if err != nil {
		return err
	}

	// 2. RAM GB-hours
	ramGB := float64(memoryBytes) / (1024 * 1024 * 1024)
	ramGBHours := ramGB * durationHours
	err = m.usageRepo.RecordUsage(ctx, &billing.UsageRecord{
		TenantID:       tenantID,
		ResourceType:   "instance",
		ResourceID:     instanceID,
		Metric:         billing.MetricRAMGBHours,
		Quantity:       ramGBHours,
		Unit:           "GB-hours",
		PeriodStart:    start,
		PeriodEnd:      end,
		IdempotencyKey: fmt.Sprintf("%s-ram", bucketKey),
		Metadata:       map[string]interface{}{"memoryBytes": memoryBytes, "hours": durationHours},
	})
	if err != nil {
		return err
	}

	// 3. Storage GB-months (730 hours per nominal month)
	storageGB := float64(storageBytes) / (1024 * 1024 * 1024)
	storageGBMonths := (storageGB * durationHours) / 730.0
	err = m.usageRepo.RecordUsage(ctx, &billing.UsageRecord{
		TenantID:       tenantID,
		ResourceType:   "instance",
		ResourceID:     instanceID,
		Metric:         billing.MetricStorageGBMonths,
		Quantity:       storageGBMonths,
		Unit:           "GB-months",
		PeriodStart:    start,
		PeriodEnd:      end,
		IdempotencyKey: fmt.Sprintf("%s-storage", bucketKey),
		Metadata:       map[string]interface{}{"storageBytes": storageBytes, "hours": durationHours},
	})
	return err
}

// RecordStorageVolumeUsage records storage volume allocation for persistent disks.
func (m *UsageMeteringEngine) RecordStorageVolumeUsage(
	ctx context.Context,
	tenantID string,
	volumeID string,
	storageBytes int64,
	start time.Time,
	end time.Time,
) error {
	durationHours := end.Sub(start).Hours()
	if durationHours <= 0 {
		return nil
	}

	storageGB := float64(storageBytes) / (1024 * 1024 * 1024)
	storageGBMonths := (storageGB * durationHours) / 730.0
	bucketKey := fmt.Sprintf("vol-%s-%d-%d", volumeID, start.Unix(), end.Unix())

	return m.usageRepo.RecordUsage(ctx, &billing.UsageRecord{
		TenantID:       tenantID,
		ResourceType:   "volume",
		ResourceID:     volumeID,
		Metric:         billing.MetricStorageGBMonths,
		Quantity:       storageGBMonths,
		Unit:           "GB-months",
		PeriodStart:    start,
		PeriodEnd:      end,
		IdempotencyKey: bucketKey,
		Metadata:       map[string]interface{}{"storageBytes": storageBytes, "hours": durationHours},
	})
}

// RecordNetworkEgress records outbound data transfer in Gigabytes.
func (m *UsageMeteringEngine) RecordNetworkEgress(
	ctx context.Context,
	tenantID string,
	instanceID string,
	egressBytes int64,
	periodStart time.Time,
	periodEnd time.Time,
) error {
	if egressBytes <= 0 {
		return nil
	}

	egressGB := float64(egressBytes) / (1024 * 1024 * 1024)
	bucketKey := fmt.Sprintf("net-%s-%d-%d", instanceID, periodStart.Unix(), periodEnd.Unix())

	return m.usageRepo.RecordUsage(ctx, &billing.UsageRecord{
		TenantID:       tenantID,
		ResourceType:   "instance",
		ResourceID:     instanceID,
		Metric:         billing.MetricNetworkEgressGB,
		Quantity:       egressGB,
		Unit:           "GB",
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		IdempotencyKey: bucketKey,
		Metadata:       map[string]interface{}{"egressBytes": egressBytes},
	})
}

// GetAggregateUsage retrieves tenant aggregate consumption across a billing interval.
func (m *UsageMeteringEngine) GetAggregateUsage(ctx context.Context, tenantID string, start, end time.Time) (*billing.UsageAggregate, error) {
	return m.usageRepo.GetAggregate(ctx, tenantID, start, end)
}
