package notification

import (
	"errors"
	"testing"

	"github.com/nicholas-fedor/shoutrrr/pkg/types"
)

type fakeShoutrrrSender struct {
	message string
	params  *types.Params
	errs    []error
}

func (f *fakeShoutrrrSender) Send(message string, params *types.Params) []error {
	f.message = message
	f.params = params
	return f.errs
}

func TestShoutrrrSend_EmptyURLSkips(t *testing.T) {
	n := &ShoutrrrNotifier{}

	if err := n.Send("Test", "Hello from Tuppr Dev"); err != nil {
		t.Fatalf("expected empty URL to skip without error, got %v", err)
	}
}

func TestShoutrrrSend_ForwardsTitleInParams(t *testing.T) {
	originalFactory := shoutrrrSenderFactory
	t.Cleanup(func() {
		shoutrrrSenderFactory = originalFactory
	})

	fakeSender := &fakeShoutrrrSender{}
	shoutrrrSenderFactory = func(string) (shoutrrrSender, error) {
		return fakeSender, nil
	}

	n := &ShoutrrrNotifier{URL: "ntfy://example/topic"}

	if err := n.Send("Tuppr Upgrade Started", "Node a is upgrading"); err != nil {
		t.Fatalf("expected send to succeed, got %v", err)
	}

	if fakeSender.message != "Node a is upgrading" {
		t.Fatalf("expected message to be forwarded, got %q", fakeSender.message)
	}
	if fakeSender.params == nil {
		t.Fatal("expected params to be forwarded")
	}

	title, ok := fakeSender.params.Title()
	if !ok {
		t.Fatal("expected title param to be set")
	}
	if title != "Tuppr Upgrade Started" {
		t.Fatalf("expected title param %q, got %q", "Tuppr Upgrade Started", title)
	}
}

func TestShoutrrrSend_ReturnsSenderErrors(t *testing.T) {
	originalFactory := shoutrrrSenderFactory
	t.Cleanup(func() {
		shoutrrrSenderFactory = originalFactory
	})

	fakeSender := &fakeShoutrrrSender{errs: []error{errors.New("boom")}}
	shoutrrrSenderFactory = func(string) (shoutrrrSender, error) {
		return fakeSender, nil
	}

	n := &ShoutrrrNotifier{URL: "ntfy://example/topic"}

	if err := n.Send("Tuppr Upgrade Started", "Node a is upgrading"); err == nil {
		t.Fatal("expected sender errors to be returned")
	}
}
