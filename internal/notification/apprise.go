package notification

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	apprise "github.com/unraid/apprise-go"
)

// AppriseNotifier sends notifications through a configured apprise-go target URL.
type AppriseNotifier struct {
	url    string
	sender *apprise.Apprise
}

// NewAppriseNotifier constructs a notifier, or returns nil when notifications
// are disabled. The URL is trimmed first: apprise-go trims it internally and
// echoes the trimmed form in errors, which the redaction has to match.
func NewAppriseNotifier(notificationURL string) (Notifier, error) {
	notificationURL = strings.TrimSpace(notificationURL)
	if notificationURL == "" {
		return nil, nil
	}

	n := &AppriseNotifier{url: notificationURL, sender: apprise.New()}
	if err := n.sender.Add(notificationURL); err != nil {
		return nil, n.redact(err)
	}
	return n, nil
}

func (a *AppriseNotifier) Send(title, message string) error {
	var opts []apprise.Option
	if title != "" {
		opts = append(opts, apprise.WithTitle(title))
	}

	if err := a.sender.Send(message, opts...); err != nil {
		return a.redact(err)
	}
	return nil
}

// redact strips credentials from apprise-go errors: the target error echoes the
// configured URL, and a transport failure is a *url.Error carrying the resolved
// API endpoint, which for most services embeds the token in its path.
func (a *AppriseNotifier) redact(err error) error {
	if target, ok := errors.AsType[*apprise.TargetError](err); ok {
		err = target.Err
	}
	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		err = fmt.Errorf("%s request failed: %w", urlErr.Op, urlErr.Err)
	}
	return errors.New(strings.ReplaceAll(err.Error(), a.url, "<redacted-url>"))
}
