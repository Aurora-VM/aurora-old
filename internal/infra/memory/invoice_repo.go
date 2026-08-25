package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/billing"
	"github.com/google/uuid"
)

// MemoryInvoiceRepo implements billing.InvoiceRepository in memory.
type MemoryInvoiceRepo struct {
	mu              sync.RWMutex
	invoices        map[string]*billing.Invoice     // id -> invoice
	idempotencyKeys map[string]string               // idempotencyKey -> invoiceId
}

func NewMemoryInvoiceRepo() *MemoryInvoiceRepo {
	return &MemoryInvoiceRepo{
		invoices:        make(map[string]*billing.Invoice),
		idempotencyKeys: make(map[string]string),
	}
}

func (r *MemoryInvoiceRepo) Create(ctx context.Context, invoice *billing.Invoice) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if invoice.IdempotencyKey != "" {
		if existingID, exists := r.idempotencyKeys[invoice.IdempotencyKey]; exists {
			if existing, ok := r.invoices[existingID]; ok {
				*invoice = *existing
				return nil
			}
		}
	}

	if invoice.ID == "" {
		invoice.ID = uuid.NewString()
	}
	if invoice.CreatedAt.IsZero() {
		invoice.CreatedAt = time.Now().UTC()
	}

	// Assign IDs to lines if missing
	for _, l := range invoice.Lines {
		if l.ID == "" {
			l.ID = uuid.NewString()
		}
		l.InvoiceID = invoice.ID
	}

	cp := *invoice
	// Deep copy lines
	var linesCopy []*billing.InvoiceLine
	for _, l := range invoice.Lines {
		lcp := *l
		linesCopy = append(linesCopy, &lcp)
	}
	cp.Lines = linesCopy

	r.invoices[invoice.ID] = &cp
	if invoice.IdempotencyKey != "" {
		r.idempotencyKeys[invoice.IdempotencyKey] = invoice.ID
	}
	return nil
}

func (r *MemoryInvoiceRepo) GetByID(ctx context.Context, id string) (*billing.Invoice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	inv, exists := r.invoices[id]
	if !exists {
		return nil, billing.ErrInvoiceNotFound
	}

	cp := *inv
	var linesCopy []*billing.InvoiceLine
	for _, l := range inv.Lines {
		lcp := *l
		linesCopy = append(linesCopy, &lcp)
	}
	cp.Lines = linesCopy
	return &cp, nil
}

func (r *MemoryInvoiceRepo) GetByIdempotencyKey(ctx context.Context, key string) (*billing.Invoice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, exists := r.idempotencyKeys[key]
	if !exists {
		return nil, billing.ErrInvoiceNotFound
	}
	return r.GetByID(ctx, id)
}

func (r *MemoryInvoiceRepo) ListByTenant(ctx context.Context, tenantID string, limit, offset int) ([]*billing.Invoice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*billing.Invoice
	for _, inv := range r.invoices {
		if inv.TenantID == tenantID {
			cp := *inv
			var linesCopy []*billing.InvoiceLine
			for _, l := range inv.Lines {
				lcp := *l
				linesCopy = append(linesCopy, &lcp)
			}
			cp.Lines = linesCopy
			result = append(result, &cp)
		}
	}

	// Sort newest first
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	if offset >= len(result) {
		return []*billing.Invoice{}, nil
	}

	end := offset + limit
	if limit <= 0 || end > len(result) {
		end = len(result)
	}

	return result[offset:end], nil
}

func (r *MemoryInvoiceRepo) ListAll(ctx context.Context, limit, offset int) ([]*billing.Invoice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*billing.Invoice
	for _, inv := range r.invoices {
		cp := *inv
		var linesCopy []*billing.InvoiceLine
		for _, l := range inv.Lines {
			lcp := *l
			linesCopy = append(linesCopy, &lcp)
		}
		cp.Lines = linesCopy
		result = append(result, &cp)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	if offset >= len(result) {
		return []*billing.Invoice{}, nil
	}

	end := offset + limit
	if limit <= 0 || end > len(result) {
		end = len(result)
	}

	return result[offset:end], nil
}

func (r *MemoryInvoiceRepo) UpdateStatus(ctx context.Context, id string, status billing.InvoiceStatus, paidAt *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	inv, exists := r.invoices[id]
	if !exists {
		return billing.ErrInvoiceNotFound
	}

	inv.Status = status
	if paidAt != nil {
		inv.PaidAt = paidAt
	}
	return nil
}
