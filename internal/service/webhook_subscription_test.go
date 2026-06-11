package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/whg517/sqlflow/internal/model"
)

func TestOutboundValidateWebhookURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"empty", "", true},
		{"valid https", "https://example.com/webhook", false},
		{"http non-local", "http://example.com/webhook", true},
		{"http localhost", "http://localhost:8080/hook", false},
		{"http 127.0.0.1", "http://127.0.0.1:9090/hook", false},
		{"ftp scheme", "ftp://example.com/hook", true},
		{"no host", "https:///path", true},
		{"too long", "https://example.com/" + string(make([]byte, 2100)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWebhookURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWebhookURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestOutboundValidateEvents(t *testing.T) {
	tests := []struct {
		name    string
		events  []string
		wantErr bool
	}{
		{"empty", []string{}, true},
		{"valid single", []string{"ticket.created"}, false},
		{"valid multiple", []string{"ticket.created", "ticket.approved", "sla.warning"}, false},
		{"invalid event", []string{"ticket.unknown"}, true},
		{"mixed", []string{"ticket.created", "invalid.event"}, true},
		{"all valid", []string{"ticket.created", "ticket.approved", "ticket.rejected", "ticket.executed", "sla.warning", "sla.breached"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEvents(tt.events)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateEvents(%v) error = %v, wantErr %v", tt.events, err, tt.wantErr)
			}
		})
	}
}

func TestOutboundGenerateSecret(t *testing.T) {
	s1, err := generateSecret()
	if err != nil {
		t.Fatalf("generateSecret() error = %v", err)
	}
	if len(s1) != 64 {
		t.Errorf("generateSecret() length = %d, want 64", len(s1))
	}
	s2, _ := generateSecret()
	if s1 == s2 {
		t.Error("two generated secrets should not be equal")
	}
}

func TestOutboundHMACSignature(t *testing.T) {
	secret := "test-secret-key"
	payload := WebhookPayload{EventType: "ticket.created", Data: map[string]string{"ticket_id": "123"}}
	body, _ := json.Marshal(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	if len(sig) != 64 {
		t.Errorf("HMAC signature length = %d, want 64", len(sig))
	}
}

func TestOutboundMaskWebhookURL(t *testing.T) {
	if maskWebhookURL("short") != "****" {
		t.Error("short URL should be masked as ****")
	}
	if maskWebhookURL("https://example.com/webhook") == "https://example.com/webhook" {
		t.Error("URL should be masked")
	}
}

func TestOutboundFormatSubscriptionNoSecret(t *testing.T) {
	result := FormatSubscriptionForResponse(&model.WebhookSubscription{
		ID: 1, Name: "test", URL: "https://example.com/hook",
		Secret: "super-secret", Events: `["ticket.created"]`,
		Enabled: true, FailureCount: 0, CreatedBy: "admin",
	})
	if _, ok := result["secret"]; ok {
		t.Error("secret should NEVER be present in FormatSubscriptionForResponse")
	}
}

func TestOutboundBoolToInt(t *testing.T) {
	if boolToInt(true) != 1 || boolToInt(false) != 0 {
		t.Error("boolToInt failed")
	}
}

func TestOutboundMaxURLLength(t *testing.T) {
	if MaxWebhookURLLength != 2048 {
		t.Errorf("MaxWebhookURLLength = %d, want 2048", MaxWebhookURLLength)
	}
}

func TestOutboundMaxAttempts(t *testing.T) {
	if WebhookMaxAttempts != 3 {
		t.Errorf("WebhookMaxAttempts = %d, want 3", WebhookMaxAttempts)
	}
}

func TestOutboundSentinelErrors(t *testing.T) {
	// ErrNotFound
	if !IsNotFoundError(ErrNotFound) {
		t.Error("ErrNotFound should match IsNotFoundError")
	}
	if IsNotFoundError(ErrValidation) {
		t.Error("ErrValidation should not match IsNotFoundError")
	}

	// ErrValidation
	if !IsValidationError(ErrValidation) {
		t.Error("ErrValidation should match IsValidationError")
	}
	if IsValidationError(ErrNotFound) {
		t.Error("ErrNotFound should not match IsValidationError")
	}

	// Wrapped errors
	wrapped := fmt.Errorf("%w: something", ErrNotFound)
	if !IsNotFoundError(wrapped) {
		t.Error("wrapped ErrNotFound should match IsNotFoundError")
	}

	wrappedVal := fmt.Errorf("%w: bad input", ErrValidation)
	if !IsValidationError(wrappedVal) {
		t.Error("wrapped ErrValidation should match IsValidationError")
	}
}
