package notification

import (
	"errors"
	"testing"
)

func TestNewShoutrrrNotifier_EmptyURLReturnsNil(t *testing.T) {
	if notifier := NewShoutrrrNotifier(""); notifier != nil {
		t.Fatalf("expected nil notifier for empty URL, got %T", notifier)
	}
}

func TestShoutrrrSend_ReturnsSenderErrors(t *testing.T) {
	n := &ShoutrrrNotifier{senderErr: errors.New("boom")}

	if err := n.Send("Tuppr Upgrade Started", "Node a is upgrading"); err == nil {
		t.Fatal("expected sender errors to be returned")
	}
}

func TestShoutrrrSend_NoSenderReturnsNil(t *testing.T) {
	n := &ShoutrrrNotifier{}

	if err := n.Send("Tuppr Upgrade Started", "Node a is upgrading"); err != nil {
		t.Fatalf("expected send to succeed, got %v", err)
	}
}
