package notification

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	apprise "github.com/unraid/apprise-go"
)

func TestNewAppriseNotifier_EmptyURLReturnsNil(t *testing.T) {
	notifier, err := NewAppriseNotifier("  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notifier != nil {
		t.Fatalf("expected nil notifier for empty URL, got %T", notifier)
	}
}

func TestNewAppriseNotifier_InvalidURLReturnsError(t *testing.T) {
	if _, err := NewAppriseNotifier("nosuchscheme://example"); err == nil {
		t.Fatal("expected an error for an unsupported target URL")
	}
}

func TestNewAppriseNotifier_TrimsURL(t *testing.T) {
	n, err := NewAppriseNotifier("discord://token@webhookid\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := n.(*AppriseNotifier).url; got != "discord://token@webhookid" {
		t.Fatalf("url not trimmed: %q", got)
	}
}

func TestAppriseRedact(t *testing.T) {
	const secretURL = "discord://token@webhookid"
	n := &AppriseNotifier{url: secretURL}

	tests := []struct {
		name    string
		err     error
		leaks   []string
		wantSub string
	}{
		{
			name:    "url echoed in message",
			err:     fmt.Errorf("%s: boom", secretURL),
			leaks:   []string{secretURL},
			wantSub: "<redacted-url>",
		},
		{
			name:    "target error wraps url error with resolved endpoint",
			err:     errors.Join(&apprise.TargetError{URL: secretURL, Err: &url.Error{Op: "Post", URL: "https://discord.com/api/webhooks/webhookid/token", Err: errors.New("connection refused")}}),
			leaks:   []string{secretURL, "webhooks/webhookid/token"},
			wantSub: "Post request failed: connection refused",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := n.redact(tt.err).Error()
			for _, leak := range tt.leaks {
				if strings.Contains(got, leak) {
					t.Fatalf("credential leaked into error: %q", got)
				}
			}
			if !strings.Contains(got, tt.wantSub) {
				t.Fatalf("expected %q in %q", tt.wantSub, got)
			}
		})
	}
}
