package alerting

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeAM is a minimal Alertmanager v2 silences endpoint: stores one silence,
// mints IDs, and mirrors the get/post surface the client uses.
type fakeAM struct {
	t        *testing.T
	silences map[string]silence
	nextID   int
	lastAuth string
}

func (f *fakeAM) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v2/silence/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.lastAuth = r.Header.Get("Authorization")
		sil, ok := f.silences[r.PathValue("id")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewEncoder(w).Encode(sil); err != nil {
			f.t.Errorf("encoding silence: %v", err)
		}
	})
	mux.HandleFunc("POST /api/v2/silences", func(w http.ResponseWriter, r *http.Request) {
		f.lastAuth = r.Header.Get("Authorization")
		var sil silence
		if err := json.NewDecoder(r.Body).Decode(&sil); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if sil.ID == "" {
			f.nextID++
			sil.ID = "sil-" + string(rune('0'+f.nextID))
		} else if _, ok := f.silences[sil.ID]; !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		sil.Status = &struct {
			State string `json:"state"`
		}{State: "active"}
		f.silences[sil.ID] = sil
		if err := json.NewEncoder(w).Encode(map[string]string{"silenceID": sil.ID}); err != nil {
			f.t.Errorf("encoding response: %v", err)
		}
	})
	return mux
}

func newFakeAM(t *testing.T) (*fakeAM, *Client) {
	t.Helper()
	am := &fakeAM{t: t, silences: map[string]silence{}}
	srv := httptest.NewServer(am.handler())
	t.Cleanup(srv.Close)
	return am, NewClient(srv.URL, map[string]string{"Authorization": "Bearer token"})
}

var testMatchers = []Matcher{{Name: "alertname", Value: "^CephMonDown$", IsRegex: true, IsEqual: true}}

func TestEnsure_CreatesSilence(t *testing.T) {
	am, c := newFakeAM(t)

	id, err := c.Ensure(t.Context(), "", testMatchers, 25*time.Minute, "tuppr/cluster", "test")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	sil, ok := am.silences[id]
	if !ok {
		t.Fatalf("silence %q not stored", id)
	}
	if sil.CreatedBy != "tuppr/cluster" {
		t.Fatalf("createdBy = %q", sil.CreatedBy)
	}
	if got := sil.EndsAt.Sub(sil.StartsAt); got != 25*time.Minute {
		t.Fatalf("silence span = %s, want 25m", got)
	}
	if am.lastAuth != "Bearer token" {
		t.Fatalf("Authorization header = %q", am.lastAuth)
	}
}

func TestEnsure_ExtendsKeepingIDAndStartsAt(t *testing.T) {
	am, c := newFakeAM(t)

	id, err := c.Ensure(t.Context(), "", testMatchers, 25*time.Minute, "tuppr/cluster", "test")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	created := am.silences[id]

	// Simulate a later reconcile extending the lease.
	c.now = func() time.Time { return time.Now().Add(10 * time.Minute) }
	newID, err := c.Ensure(t.Context(), id, testMatchers, 25*time.Minute, "tuppr/cluster", "test")
	if err != nil {
		t.Fatalf("extend: %v", err)
	}
	if newID != id {
		t.Fatalf("extend changed ID: %q -> %q", id, newID)
	}
	extended := am.silences[id]
	if !extended.StartsAt.Equal(created.StartsAt) {
		t.Fatalf("extend changed startsAt: %s -> %s", created.StartsAt, extended.StartsAt)
	}
	if !extended.EndsAt.After(created.EndsAt) {
		t.Fatalf("endsAt not extended: %s -> %s", created.EndsAt, extended.EndsAt)
	}
}

func TestEnsure_RecreatesWhenUnknown(t *testing.T) {
	am, c := newFakeAM(t)

	id, err := c.Ensure(t.Context(), "gone", testMatchers, 25*time.Minute, "tuppr/cluster", "test")
	if err != nil {
		t.Fatalf("ensure with unknown id: %v", err)
	}
	if id == "gone" || id == "" {
		t.Fatalf("expected a fresh silence ID, got %q", id)
	}
	if _, ok := am.silences[id]; !ok {
		t.Fatalf("fresh silence %q not stored", id)
	}
}

func TestExpire_ShortensToTail(t *testing.T) {
	am, c := newFakeAM(t)

	id, err := c.Ensure(t.Context(), "", testMatchers, 25*time.Minute, "tuppr/cluster", "test")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := c.Expire(t.Context(), id, 5*time.Minute); err != nil {
		t.Fatalf("expire: %v", err)
	}
	got := time.Until(am.silences[id].EndsAt)
	if got > 5*time.Minute || got < 4*time.Minute {
		t.Fatalf("endsAt %s from now, want ~5m", got)
	}
}

func TestExpire_UnknownSilenceIsNotAnError(t *testing.T) {
	_, c := newFakeAM(t)
	if err := c.Expire(t.Context(), "gone", 5*time.Minute); err != nil {
		t.Fatalf("expire unknown: %v", err)
	}
}

func TestNewMatcher_MapsOperators(t *testing.T) {
	tests := []struct {
		matchType string
		isRegex   bool
		isEqual   bool
	}{
		{"=", false, true},
		{"", false, true},
		{"!=", false, false},
		{"=~", true, true},
		{"!~", true, false},
	}
	for _, tt := range tests {
		m := NewMatcher("l", "v", tt.matchType)
		if m.IsRegex != tt.isRegex || m.IsEqual != tt.isEqual {
			t.Errorf("NewMatcher(%q) = {regex:%v equal:%v}, want {regex:%v equal:%v}",
				tt.matchType, m.IsRegex, m.IsEqual, tt.isRegex, tt.isEqual)
		}
	}
}

func TestLoadHeadersDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "X-Scope-OrgID"), []byte("tenant-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Kubernetes atomic-update machinery must be skipped.
	if err := os.Mkdir(filepath.Join(dir, "..2026_01_01"), 0o700); err != nil {
		t.Fatal(err)
	}

	headers, err := LoadHeadersDir(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(headers) != 1 || headers["X-Scope-OrgID"] != "tenant-1" {
		t.Fatalf("headers = %v", headers)
	}

	if h, err := LoadHeadersDir(""); err != nil || h != nil {
		t.Fatalf("empty dir: %v, %v", h, err)
	}
}
