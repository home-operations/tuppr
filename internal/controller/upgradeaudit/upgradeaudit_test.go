package upgradeaudit

import "testing"

func TestPrependHistory_PrependsAndCaps(t *testing.T) {
	history := []int{2, 3, 4, 5}
	got := PrependHistory(history, 1, 3)
	if len(got) != 3 {
		t.Fatalf("want len 3, got %d", len(got))
	}
	if got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("want [1 2 3], got %v", got)
	}
}

func TestPrependHistory_EmptyHistory(t *testing.T) {
	var empty []string
	got := PrependHistory(empty, "a", 5)
	if len(got) != 1 || got[0] != "a" {
		t.Fatalf("want [a], got %v", got)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := FirstNonEmpty("", "", "x", "y"); got != "x" {
		t.Fatalf("want x, got %q", got)
	}
	if got := FirstNonEmpty("", ""); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}
