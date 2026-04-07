package notification

import (
	"errors"

	"github.com/nicholas-fedor/shoutrrr"
	"github.com/nicholas-fedor/shoutrrr/pkg/types"
)

// ShoutrrrNotifier sends notifications through a configured Shoutrrr URL.
type ShoutrrrNotifier struct {
	URL string
}

type shoutrrrSender interface {
	Send(message string, params *types.Params) []error
}

// shourtrrrSenderFactory is overridden in tests.
var shoutrrrSenderFactory = newShoutrrrSender

func newShoutrrrSender(url string) (shoutrrrSender, error) {
	return shoutrrr.CreateSender(url)
}

func (s *ShoutrrrNotifier) Send(title, message string) error {
	if s.URL == "" {
		return nil
	}

	sender, err := shoutrrrSenderFactory(s.URL)
	if err != nil {
		return err
	}

	params := types.Params{}
	if title != "" {
		params.SetTitle(title)
	}

	return errors.Join(sender.Send(message, &params)...)
}
