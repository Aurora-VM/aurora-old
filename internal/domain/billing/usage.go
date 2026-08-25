package billing

import (
	"time"
)

type MetricType string

const (
	MetricVCPUHours       MetricType = "vcpu_hours"
	MetricRAMGBHours      MetricType = "ram_gb_hours"
	MetricStorageGBMonths MetricType = "storage_gb_months"
	MetricIPv4Addresses   MetricType = "ipv4_addresses"
	MetricSnapshotCount   MetricType = "snapshot_count"
	MetricBackupStorageMB MetricType = "backup_storage_mb"
	MetricNetworkEgressGB MetricType = "network_egress_gb"
	MetricInstanceCount   MetricType = "instance_count"
)

// UsageRecord represents an atomic, idempotent billable event or periodic usage increment.
type UsageRecord struct {
	ID             string                 `json:"id"`
	TenantID       string                 `json:"tenantId"`
	ResourceType   string                 `json:"resourceType"` // e.g. "instance", "volume", "ip"
	ResourceID     string                 `json:"resourceId"`
	Metric         MetricType             `json:"metric"`
	Quantity       float64                `json:"quantity"`
	Unit           string                 `json:"unit"`
	PeriodStart    time.Time              `json:"periodStart"`
	PeriodEnd      time.Time              `json:"periodEnd"`
	IdempotencyKey string                 `json:"idempotencyKey,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt      time.Time              `json:"createdAt"`
}

// UsageAggregate groups total consumed quantity by metric for a given tenant within a billing period.
type UsageAggregate struct {
	TenantID    string                `json:"tenantId"`
	PeriodStart time.Time             `json:"periodStart"`
	PeriodEnd   time.Time             `json:"periodEnd"`
	Metrics     map[MetricType]float64 `json:"metrics"`
}
