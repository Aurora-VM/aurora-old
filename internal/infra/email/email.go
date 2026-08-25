package email

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/events"
)

// SentEmail records an outgoing email message for inspection and audit.
type SentEmail struct {
	ID        string    `json:"id"`
	To        string    `json:"to"`
	Subject   string    `json:"subject"`
	HTMLBody  string    `json:"htmlBody"`
	TextBody  string    `json:"textBody"`
	EventType string    `json:"eventType,omitempty"`
	SentAt    time.Time `json:"sentAt"`
}

// EmailProvider defines the interface for delivering email notifications.
type EmailProvider interface {
	SendEmail(ctx context.Context, to string, subject string, htmlBody string, textBody string) error
	SendEventNotification(ctx context.Context, to string, event *events.Event) error
	GetSentEmails() []SentEmail
	Clear()
}

// SimulatedEmailProvider provides a thread-safe in-memory email driver.
type SimulatedEmailProvider struct {
	mu     sync.RWMutex
	emails []SentEmail
}

func NewSimulatedEmailProvider() *SimulatedEmailProvider {
	return &SimulatedEmailProvider{
		emails: make([]SentEmail, 0),
	}
}

func (p *SimulatedEmailProvider) SendEmail(ctx context.Context, to string, subject string, htmlBody string, textBody string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	msg := SentEmail{
		ID:       fmt.Sprintf("email-%d", time.Now().UnixNano()),
		To:       to,
		Subject:  subject,
		HTMLBody: htmlBody,
		TextBody: textBody,
		SentAt:   time.Now().UTC(),
	}
	p.emails = append(p.emails, msg)
	return nil
}

func (p *SimulatedEmailProvider) SendEventNotification(ctx context.Context, to string, event *events.Event) error {
	subject := fmt.Sprintf("[Aurora Cloud] %s notification for %s", string(event.Type), event.ResourceID)
	textBody := fmt.Sprintf(
		"Hello,\n\nAn event occurred in your Project Aurora tenant:\n- Event: %s\n- Resource: %s (%s)\n- Timestamp: %s\n\nBest regards,\nAurora Cloud Platform",
		event.Type, event.ResourceID, event.ResourceType, event.Timestamp.Format(time.RFC3339),
	)
	htmlBody := fmt.Sprintf(
		`<div style="font-family: sans-serif; padding: 20px;">
			<h2>Aurora Cloud Notification</h2>
			<p><strong>Event:</strong> %s</p>
			<p><strong>Resource:</strong> %s (%s)</p>
			<p><strong>Time:</strong> %s</p>
		</div>`,
		event.Type, event.ResourceID, event.ResourceType, event.Timestamp.Format(time.RFC3339),
	)

	p.mu.Lock()
	defer p.mu.Unlock()

	msg := SentEmail{
		ID:        fmt.Sprintf("email-%d", time.Now().UnixNano()),
		To:        to,
		Subject:   subject,
		HTMLBody:  htmlBody,
		TextBody:  textBody,
		EventType: string(event.Type),
		SentAt:    time.Now().UTC(),
	}
	p.emails = append(p.emails, msg)
	return nil
}

func (p *SimulatedEmailProvider) GetSentEmails() []SentEmail {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]SentEmail, len(p.emails))
	copy(result, p.emails)
	return result
}

func (p *SimulatedEmailProvider) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.emails = make([]SentEmail, 0)
}
