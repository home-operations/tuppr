package upgradeaudit

// HistoryMaxEntries bounds the per-resource upgrade history slice. Both
// TalosUpgrade and KubernetesUpgrade keep this many of their most recent
// terminal transitions.
const HistoryMaxEntries = 10

// PrependHistory inserts entry at the front of history and truncates to max.
// The element type is left generic so each controller can use its own history
// entry struct without losing type safety.
func PrependHistory[T any](history []T, entry T, max int) []T {
	next := make([]T, 0, min(len(history)+1, max))
	next = append(next, entry)
	for i := 0; i < len(history) && len(next) < max; i++ {
		next = append(next, history[i])
	}
	return next
}

// FirstNonEmpty returns the first non-empty value or "" if all are empty.
func FirstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
