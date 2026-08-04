package notification

import (
	"fmt"
	"strings"
	"testing"
)

func TestNewAppriseNotifier_EmptyURLReturnsNil(t *testing.T) {
	if notifier := NewAppriseNotifier(""); notifier != nil {
		t.Fatalf("expected nil notifier for empty URL, got %T", notifier)
	}
}

func TestNewAppriseNotifier_InvalidURLSurfacesErrorOnSend(t *testing.T) {
	n := NewAppriseNotifier("nosuchscheme://example")
	if n == nil {
		t.Fatal("expected a notifier for a non-empty URL")
	}

	if err := n.Send("Tuppr Upgrade Started", "Node a is upgrading"); err == nil {
		t.Fatal("expected an error for an unsupported target URL")
	}
}

func TestAppriseSend_RedactsCredentialURL(t *testing.T) {
	secretURL := "discord://token@webhookid"
	n := &AppriseNotifier{url: secretURL, addErr: fmt.Errorf("%s: boom", secretURL)}

	err := n.Send("Tuppr Upgrade Started", "Node a is upgrading")
	if err == nil {
		t.Fatal("expected the stored error to be returned")
	}
	if strings.Contains(err.Error(), secretURL) {
		t.Fatalf("credential URL leaked into error: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "<redacted-url>") {
		t.Fatalf("expected redaction placeholder, got %q", err.Error())
	}
}
