package billing

import "errors"

var (
	ErrPlanNotFound             = errors.New("billing plan not found")
	ErrPlanInactive             = errors.New("billing plan is inactive")
	ErrPlanSlugExists           = errors.New("billing plan slug already exists")
	ErrInvalidPlanSpec          = errors.New("invalid plan specification")
	ErrSubscriptionNotFound     = errors.New("subscription not found")
	ErrSubscriptionAlreadyExists = errors.New("tenant already has an active subscription")
	ErrInvalidSubscription      = errors.New("invalid subscription specification")
	ErrQuotaExceeded            = errors.New("resource quota exceeded")
	ErrQuotaNotFound            = errors.New("quota not found")
	ErrInvoiceNotFound          = errors.New("invoice not found")
	ErrInvalidInvoiceState      = errors.New("invalid invoice state transition")
	ErrDuplicateIdempotencyKey  = errors.New("duplicate idempotency key")
	ErrPaymentFailed            = errors.New("payment processing failed")
)
