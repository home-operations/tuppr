package notification

import (
	"errors"

	"github.com/containrrr/shoutrrr"
	"github.com/containrrr/shoutrrr/pkg/types"
)

// ShoutrrrNotifier sends notifications through a configured Shoutrrr URL.
type ShoutrrrNotifier struct {
	URL string
}

type shoutrrrSender interface {
	Send(message string, params *types.Params) []error
}

var newShoutrrrSender = func(url string) (shoutrrrSender, error) {
	return shoutrrr.CreateSender(url)
}

func (s *ShoutrrrNotifier) Send(title, message string) error {
	if s.URL == "" {
		return nil
	}

	sender, err := newShoutrrrSender(s.URL)
	if err != nil {
		return err
	}

	params := types.Params{}
	if title != "" {
		params.SetTitle(title)
	}

	return errors.Join(sender.Send(message, &params)...)
}
