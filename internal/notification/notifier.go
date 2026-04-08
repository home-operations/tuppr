package notification

// Notifier defines how tuppr sends notifications.

type Notifier interface {
	Send(title, message string) error
}
