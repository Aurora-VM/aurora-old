package webhook_test

import (
	"strings"
	"testing"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/webhook"
)

func TestWebhook_SecretGenerationAndRotation(t *testing.T) {
	secret1 := webhook.GenerateSecret()
	if !strings.HasPrefix(secret1, "whsec_") || len(secret1) < 40 {
		t.Fatalf("expected valid secret starting with whsec_, got %s", secret1)
	}

	ep := &webhook.WebhookEndpoint{
		ID:     "ep-01",
		Secret: secret1,
	}

	secret2 := ep.RotateSecret()
	if secret2 == secret1 {
		t.Errorf("expected new secret on rotation, got duplicate %s", secret2)
	}
	if ep.Secret != secret2 {
		t.Errorf("expected endpoint secret to be updated, got %s", ep.Secret)
	}
}

func TestWebhook_HMACSigningAndVerification(t *testing.T) {
	secret := webhook.GenerateSecret()
	payload := []byte(`{"id":"evt-123","type":"instance.created","tenantId":"tenant-01"}`)
	ts := time.Now().UTC().Unix()

	sig := webhook.SignPayload(secret, ts, payload)
	if len(sig) != 64 { // SHA-256 hex string is 64 characters
		t.Fatalf("expected 64 char hex signature, got %d chars: %s", len(sig), sig)
	}

	// 1. Valid verification with raw hex
	valid := webhook.VerifySignature(secret, sig, ts, payload, 5*time.Minute)
	if !valid {
		t.Errorf("expected signature to verify successfully")
	}

	// 2. Valid verification with sha256= prefix
	validPrefixed := webhook.VerifySignature(secret, "sha256="+sig, ts, payload, 5*time.Minute)
	if !validPrefixed {
		t.Errorf("expected sha256= prefixed signature to verify successfully")
	}

	// 3. Tampered payload fails verification
	tamperedPayload := []byte(`{"id":"evt-123","type":"instance.deleted","tenantId":"tenant-01"}`)
	if webhook.VerifySignature(secret, sig, ts, tamperedPayload, 5*time.Minute) {
		t.Errorf("expected tampered payload to fail verification")
	}

	// 4. Invalid secret fails verification
	if webhook.VerifySignature("whsec_wrongsecret1234567890abcdef", sig, ts, payload, 5*time.Minute) {
		t.Errorf("expected wrong secret to fail verification")
	}

	// 5. Expired timestamp beyond tolerance fails (Replay attack protection)
	oldTimestamp := time.Now().Add(-10 * time.Minute).Unix()
	oldSig := webhook.SignPayload(secret, oldTimestamp, payload)
	if webhook.VerifySignature(secret, oldSig, oldTimestamp, payload, 5*time.Minute) {
		t.Errorf("expected expired timestamp to fail verification due to tolerance limit")
	}
}

func TestWebhook_WildcardSubscriptionMatching(t *testing.T) {
	ep := &webhook.WebhookEndpoint{
		Active:     true,
		EventTypes: []string{"instance.*", "billing.invoice.paid"},
	}

	if !ep.SubscribesTo("instance.created") {
		t.Errorf("expected instance.created to match instance.*")
	}
	if !ep.SubscribesTo("instance.deleted") {
		t.Errorf("expected instance.deleted to match instance.*")
	}
	if !ep.SubscribesTo("billing.invoice.paid") {
		t.Errorf("expected exact match on billing.invoice.paid")
	}
	if ep.SubscribesTo("billing.invoice.created") {
		t.Errorf("did not expect billing.invoice.created to match")
	}
	if ep.SubscribesTo("node.enrolled") {
		t.Errorf("did not expect node.enrolled to match")
	}

	// Inactive endpoint receives nothing
	ep.Active = false
	if ep.SubscribesTo("instance.created") {
		t.Errorf("inactive endpoint should not subscribe to anything")
	}
}
