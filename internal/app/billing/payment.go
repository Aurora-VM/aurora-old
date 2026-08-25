package billing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/billing"
	"github.com/google/uuid"
)

// SimulatedPaymentProvider simulates an upstream payment gateway (e.g. Stripe/Adyen) for test and demo mode.
type SimulatedPaymentProvider struct {
	mu          sync.RWMutex
	customers   map[string]string // tenantID -> customerID
	payments    map[string]*billing.PaymentResult
	idempotency map[string]string // idempotencyKey -> paymentID
}

func NewSimulatedPaymentProvider() *SimulatedPaymentProvider {
	return &SimulatedPaymentProvider{
		customers:   make(map[string]string),
		payments:    make(map[string]*billing.PaymentResult),
		idempotency: make(map[string]string),
	}
}

func (p *SimulatedPaymentProvider) CreateCustomer(ctx context.Context, tenantID, email, name string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if cid, exists := p.customers[tenantID]; exists {
		return cid, nil
	}
	custID := "cus_" + uuid.NewString()[:12]
	p.customers[tenantID] = custID
	return custID, nil
}

func (p *SimulatedPaymentProvider) CreatePayment(
	ctx context.Context,
	idempotencyKey string,
	amountMinor int64,
	currency string,
	description string,
) (*billing.PaymentResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if idempotencyKey != "" {
		if pid, exists := p.idempotency[idempotencyKey]; exists {
			if existing, ok := p.payments[pid]; ok {
				return existing, nil
			}
		}
	}

	paymentID := "pay_" + uuid.NewString()[:12]
	res := &billing.PaymentResult{
		PaymentID:      paymentID,
		IdempotencyKey: idempotencyKey,
		AmountMinor:    amountMinor,
		Currency:       currency,
		Status:         "succeeded",
		CreatedAt:      time.Now().UTC(),
	}

	p.payments[paymentID] = res
	if idempotencyKey != "" {
		p.idempotency[idempotencyKey] = paymentID
	}
	return res, nil
}

func (p *SimulatedPaymentProvider) Refund(ctx context.Context, paymentID string, amountMinor int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	pay, exists := p.payments[paymentID]
	if !exists {
		return fmt.Errorf("payment %s not found", paymentID)
	}
	pay.Status = "refunded"
	return nil
}

func (p *SimulatedPaymentProvider) GetPaymentStatus(ctx context.Context, paymentID string) (*billing.PaymentResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	pay, exists := p.payments[paymentID]
	if !exists {
		return nil, fmt.Errorf("payment %s not found", paymentID)
	}
	return pay, nil
}
