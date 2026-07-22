package notification

import (
	"errors"
	"strings"

	apprise "github.com/unraid/apprise-go"
)

// AppriseNotifier sends notifications through a configured apprise-go target URL.
type AppriseNotifier struct {
	url    string
	sender *apprise.Apprise
	addErr error
}

// NewAppriseNotifier constructs a notifier or returns nil when notifications are disabled.
func NewAppriseNotifier(notificationURL string) Notifier {
	if notificationURL == "" {
		return nil
	}

	sender := apprise.New()

	return &AppriseNotifier{
		url:    notificationURL,
		sender: sender,
		addErr: sender.Add(notificationURL),
	}
}

func (a *AppriseNotifier) Send(title, message string) error {
	if a.addErr != nil {
		return a.redact(a.addErr)
	}

	var opts []apprise.Option
	if title != "" {
		opts = append(opts, apprise.WithTitle(title))
	}

	if err := a.sender.Send(message, opts...); err != nil {
		return a.redact(err)
	}
	return nil
}

// redact strips the credential-bearing target URL that apprise-go echoes in its
// error text, so it can't reach logs.
func (a *AppriseNotifier) redact(err error) error {
	if a.url == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), a.url, "<redacted-url>"))
}
