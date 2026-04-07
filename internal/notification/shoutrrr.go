package notification

import (
	"errors"

	"github.com/nicholas-fedor/shoutrrr"
	"github.com/nicholas-fedor/shoutrrr/pkg/types"
)

// ShoutrrrNotifier sends notifications through a configured Shoutrrr URL.
type ShoutrrrNotifier struct {
	sender    shoutrrrSender
	senderErr error
}

type shoutrrrSender interface {
	Send(message string, params *types.Params) []error
}

// NewShoutrrrNotifier constructs a notifier or returns nil when notifications are disabled.
func NewShoutrrrNotifier(notificationURL string) Notifier {
	if notificationURL == "" {
		return nil
	}

	sender, err := shoutrrr.CreateSender(notificationURL)

	return &ShoutrrrNotifier{
		sender:    sender,
		senderErr: err,
	}
}

func (s *ShoutrrrNotifier) Send(title, message string) error {
	if s.senderErr != nil {
		return s.senderErr
	}

	if s.sender == nil {
		return nil
	}

	params := types.Params{}
	if title != "" {
		params.SetTitle(title)
	}

	return errors.Join(s.sender.Send(message, &params)...)
}
