// Package alerting manages Alertmanager silences over the v2 silences API.
// Silences are held as short leases: Ensure creates or re-extends one, Expire
// shortens it to a small tail, and a silence whose holder stops calling Ensure
// simply lapses at its endsAt.
package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Silencer is the controller-facing surface, kept minimal for mocking.
type Silencer interface {
	// Ensure creates the silence (empty id) or extends its endsAt to now+ttl.
	// It returns the silence ID to persist, which may differ from id when
	// Alertmanager replaced the silence (expired, deleted, or matchers changed).
	Ensure(ctx context.Context, id string, matchers []Matcher, ttl time.Duration, createdBy, comment string) (string, error)

	// Expire shortens the silence to end at now+tail, so the alert tail of the
	// just-finished disruption stays covered without holding the silence open.
	// Expiring an unknown or already-expired silence is not an error.
	Expire(ctx context.Context, id string, tail time.Duration) error
}

// Matcher is one Alertmanager silence matcher in v2 API form.
type Matcher struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	IsRegex bool   `json:"isRegex"`
	IsEqual bool   `json:"isEqual"`
}

// NewMatcher maps a CRD matchType operator (=, !=, =~, !~) to a v2 matcher.
func NewMatcher(name, value, matchType string) Matcher {
	m := Matcher{Name: name, Value: value, IsEqual: true}
	switch matchType {
	case "!=":
		m.IsEqual = false
	case "=~":
		m.IsRegex = true
	case "!~":
		m.IsRegex = true
		m.IsEqual = false
	}
	return m
}

type silence struct {
	ID        string    `json:"id,omitempty"`
	Matchers  []Matcher `json:"matchers"`
	StartsAt  time.Time `json:"startsAt"`
	EndsAt    time.Time `json:"endsAt"`
	CreatedBy string    `json:"createdBy"`
	Comment   string    `json:"comment"`
	Status    *struct {
		State string `json:"state"`
	} `json:"status,omitempty"`
}

// Client talks to one Alertmanager (or any v2-compatible endpoint: VMAlertmanager,
// Grafana-managed alerting behind its prefix).
type Client struct {
	baseURL string
	headers map[string]string
	hc      *http.Client
	now     func() time.Time
}

// NewClient builds a Client for the given base URL. Every request carries the
// given headers (e.g. Authorization, X-Scope-OrgID).
func NewClient(baseURL string, headers map[string]string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		headers: headers,
		hc:      &http.Client{Timeout: 10 * time.Second},
		now:     time.Now,
	}
}

func (c *Client) Ensure(ctx context.Context, id string, matchers []Matcher, ttl time.Duration, createdBy, comment string) (string, error) {
	now := c.now()
	sil := silence{
		Matchers:  matchers,
		StartsAt:  now,
		EndsAt:    now.Add(ttl),
		CreatedBy: createdBy,
		Comment:   comment,
	}

	if id != "" {
		// Extending in place requires the original startsAt; a changed startsAt
		// makes Alertmanager expire the old silence and mint a new ID instead.
		if existing, err := c.get(ctx, id); err != nil {
			return "", err
		} else if existing != nil && (existing.Status == nil || existing.Status.State != "expired") {
			sil.ID = id
			sil.StartsAt = existing.StartsAt
		}
	}

	return c.post(ctx, sil)
}

func (c *Client) Expire(ctx context.Context, id string, tail time.Duration) error {
	existing, err := c.get(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil || (existing.Status != nil && existing.Status.State == "expired") {
		return nil
	}

	// Keep Alertmanager's own matchers so the update stays in place even if the
	// spec's matchers changed since creation.
	existing.EndsAt = c.now().Add(tail)
	existing.Status = nil
	_, err = c.post(ctx, *existing)
	return err
}

// get returns the silence, or nil when Alertmanager doesn't know the ID.
func (c *Client) get(ctx context.Context, id string) (*silence, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v2/silence/"+id, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck

	switch resp.StatusCode {
	case http.StatusOK:
		var sil silence
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&sil); err != nil {
			return nil, fmt.Errorf("decoding silence %s: %w", id, err)
		}
		return &sil, nil
	case http.StatusNotFound:
		return nil, nil
	default:
		return nil, fmt.Errorf("getting silence %s: unexpected status %s", id, resp.Status)
	}
}

func (c *Client) post(ctx context.Context, sil silence) (string, error) {
	body, err := json.Marshal(sil)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v2/silences", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return "", fmt.Errorf("posting silence: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	var result struct {
		SilenceID string `json:"silenceID"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding silence response: %w", err)
	}
	return result.SilenceID, nil
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	return c.hc.Do(req)
}

// LoadHeadersDir reads a directory of files into a header map (file name =
// header name, trimmed contents = value) — the shape of a mounted Secret
// volume. A missing or empty dir yields nil. Kubernetes' atomic-update
// machinery ("..data" and friends) is skipped.
func LoadHeadersDir(dir string) (map[string]string, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading alertmanager headers dir: %w", err)
	}
	headers := map[string]string{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "..") || e.IsDir() {
			continue
		}
		value, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading alertmanager header %s: %w", e.Name(), err)
		}
		headers[e.Name()] = strings.TrimSpace(string(value))
	}
	if len(headers) == 0 {
		return nil, nil
	}
	return headers, nil
}
