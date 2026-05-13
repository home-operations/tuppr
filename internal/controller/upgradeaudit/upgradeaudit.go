package upgradeaudit

const HistoryMaxEntries = 10

// MaxCompletionCycles caps Completed→Pending re-entries before the run is
// marked Failed. Each cycle adds one history entry, so this must stay below
// HistoryMaxEntries.
const MaxCompletionCycles = 5

func PrependHistory[T any](history []T, entry T, limit int) []T {
	next := make([]T, 0, min(len(history)+1, limit))
	next = append(next, entry)
	for i := 0; i < len(history) && len(next) < limit; i++ {
		next = append(next, history[i])
	}
	return next
}

func FirstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
