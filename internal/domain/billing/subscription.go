package billing

import (
	"fmt"
	"time"
)

type SubscriptionStatus string

const (
	SubscriptionStatusActive     SubscriptionStatus = "active"
	SubscriptionStatusCanceled   SubscriptionStatus = "canceled"
	SubscriptionStatusPastDue    SubscriptionStatus = "past_due"
	SubscriptionStatusIncomplete SubscriptionStatus = "incomplete"
	SubscriptionStatusTrialing   SubscriptionStatus = "trialing"
)

type BillingCycle string

const (
	BillingCycleMonthly BillingCycle = "monthly"
	BillingCycleYearly  BillingCycle = "yearly"
)

// Subscription represents a tenant's binding to a billing plan.
type Subscription struct {
	ID                 string             `json:"id"`
	TenantID           string             `json:"tenantId"` // UserID / TenantID
	PlanID             string             `json:"planId"`
	Status             SubscriptionStatus `json:"status"`
	BillingCycle       BillingCycle       `json:"billingCycle"`
	CurrentPeriodStart time.Time          `json:"currentPeriodStart"`
	CurrentPeriodEnd   time.Time          `json:"currentPeriodEnd"`
	CancelAtPeriodEnd  bool               `json:"cancelAtPeriodEnd"`
	CreatedAt          time.Time          `json:"createdAt"`
	UpdatedAt          time.Time          `json:"updatedAt"`
}

func (s *Subscription) Validate() error {
	if s.TenantID == "" {
		return fmt.Errorf("%w: tenant ID is required", ErrInvalidSubscription)
	}
	if s.PlanID == "" {
		return fmt.Errorf("%w: plan ID is required", ErrInvalidSubscription)
	}
	if s.BillingCycle != BillingCycleMonthly && s.BillingCycle != BillingCycleYearly {
		return fmt.Errorf("%w: invalid billing cycle (must be monthly or yearly)", ErrInvalidSubscription)
	}
	if s.CurrentPeriodEnd.Before(s.CurrentPeriodStart) {
		return fmt.Errorf("%w: period end cannot precede period start", ErrInvalidSubscription)
	}
	return nil
}
