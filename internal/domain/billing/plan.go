package billing

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)

// Plan represents a product-level tier/package defining included resources, caps, and pricing.
type Plan struct {
	ID                  string            `json:"id"`
	Name                string            `json:"name"`
	Slug                string            `json:"slug"`
	Description         string            `json:"description"`
	Currency            string            `json:"currency"` // Standard ISO 4217, e.g. "EUR", "USD"
	MonthlyPriceMinor   int64             `json:"monthlyPriceMinor"`
	YearlyPriceMinor    int64             `json:"yearlyPriceMinor"`
	IncludedVCPU        int               `json:"includedVcpu"`
	IncludedMemoryMB    int64             `json:"includedMemoryMb"`
	IncludedStorageMB   int64             `json:"includedStorageMb"`
	IncludedIPv4        int               `json:"includedIpv4"`
	IncludedSnapshots   int               `json:"includedSnapshots"`
	IncludedBackups     int               `json:"includedBackups"`
	IncludedBandwidthGB int64             `json:"includedBandwidthGb"`
	MaximumInstances    int               `json:"maximumInstances"`
	MaximumVCPU         int               `json:"maximumVcpu"`
	MaximumMemoryMB     int64             `json:"maximumMemoryMb"`
	MaximumStorageMB    int64             `json:"maximumStorageMb"`
	Features            map[string]bool   `json:"features"`
	Active              bool              `json:"active"`
	CreatedAt           time.Time         `json:"createdAt"`
	UpdatedAt           time.Time         `json:"updatedAt"`
}

// Validate checks plan domain constraints.
func (p *Plan) Validate() error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || len(p.Name) > 100 {
		return fmt.Errorf("%w: plan name must be between 1 and 100 characters", ErrInvalidPlanSpec)
	}

	p.Slug = strings.ToLower(strings.TrimSpace(p.Slug))
	if !slugRegex.MatchString(p.Slug) {
		return fmt.Errorf("%w: invalid slug '%s' (lowercase alphanumeric with hyphens)", ErrInvalidPlanSpec, p.Slug)
	}

	p.Currency = strings.ToUpper(strings.TrimSpace(p.Currency))
	if len(p.Currency) != 3 {
		return fmt.Errorf("%w: currency must be 3-letter ISO code", ErrInvalidPlanSpec)
	}

	if p.MonthlyPriceMinor < 0 || p.YearlyPriceMinor < 0 {
		return fmt.Errorf("%w: prices cannot be negative", ErrInvalidPlanSpec)
	}

	if p.MaximumInstances < 0 || p.MaximumVCPU < 0 || p.MaximumMemoryMB < 0 || p.MaximumStorageMB < 0 {
		return fmt.Errorf("%w: maximum limits cannot be negative", ErrInvalidPlanSpec)
	}

	if p.IncludedVCPU < 0 || p.IncludedMemoryMB < 0 || p.IncludedStorageMB < 0 || p.IncludedIPv4 < 0 {
		return fmt.Errorf("%w: included allowances cannot be negative", ErrInvalidPlanSpec)
	}

	return nil
}
