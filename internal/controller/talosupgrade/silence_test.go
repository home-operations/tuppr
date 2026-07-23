package talosupgrade

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/alerting"
)

type mockSilencer struct {
	ensureCalls  int
	expireCalls  int
	ensureIDs    []string
	lastMatchers []alerting.Matcher
	expiredIDs   []string
	returnIDs    []string
	ensureErr    error
	expireErr    error
}

func (m *mockSilencer) Ensure(_ context.Context, id string, matchers []alerting.Matcher, _ time.Duration, _, _ string) (string, error) {
	m.ensureCalls++
	m.ensureIDs = append(m.ensureIDs, id)
	m.lastMatchers = matchers
	if m.ensureErr != nil {
		return "", m.ensureErr
	}
	if len(m.returnIDs) > 0 {
		next := m.returnIDs[0]
		m.returnIDs = m.returnIDs[1:]
		return next, nil
	}
	return id, nil
}

func (m *mockSilencer) Expire(_ context.Context, id string, _ time.Duration) error {
	m.expireCalls++
	m.expiredIDs = append(m.expiredIDs, id)
	return m.expireErr
}

func withSilences(tu *tupprv1alpha1.TalosUpgrade) {
	tu.Spec.Silences = []tupprv1alpha1.SilenceSpec{
		{
			Matchers: []tupprv1alpha1.SilenceMatcher{
				{Name: "alertname", Value: "^(CephMonDown|CephOSDDown)$", MatchType: "=~"},
			},
		},
	}
}

const (
	testSilenceID  = "sil-1"
	testSilenceID2 = "sil-2"
)

func withHeldSilence(tu *tupprv1alpha1.TalosUpgrade) {
	tu.Status.AlertSilenceIDs = []string{testSilenceID}
}

func withSilencesSince(t time.Time) func(*tupprv1alpha1.TalosUpgrade) {
	return func(tu *tupprv1alpha1.TalosUpgrade) {
		since := metav1.NewTime(t)
		tu.Status.AlertSilencesSince = &since
	}
}

func newSilenceReconciler(t *testing.T, tu *tupprv1alpha1.TalosUpgrade, silencer alerting.Silencer) *Reconciler {
	t.Helper()
	scheme := newTestScheme()
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu).WithStatusSubresource(tu).Build()
	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})
	r.Silencer = silencer
	return r
}

func TestSyncAlertSilences_NilSilencerWarnsOnce(t *testing.T) {
	tu := newTalosUpgrade("test", withSilences, withPhase(tupprv1alpha1.JobPhaseUpgrading))
	r := newSilenceReconciler(t, tu, nil)
	recorder := record.NewFakeRecorder(4)
	r.Recorder = recorder

	// The warning is latched: repeated reconciles emit it exactly once.
	r.syncAlertSilences(context.Background(), tu, false)
	r.syncAlertSilences(context.Background(), tu, false)

	events := drainEvents(recorder)
	if len(events) != 1 || !strings.Contains(events[0], "SilencesNotConfigured") {
		t.Fatalf("expected exactly one SilencesNotConfigured event, got %v", events)
	}
}

func TestSyncAlertSilences_NilSilencerClearsStaleIDs(t *testing.T) {
	// Operator redeployed without silences while a run held IDs: the silences
	// lapsed at their TTL, so the stale status entries are cleared.
	tu := newTalosUpgrade("test", withSilences,
		withPhase(tupprv1alpha1.JobPhaseUpgrading), withHeldSilence)
	r := newSilenceReconciler(t, tu, nil)

	r.syncAlertSilences(context.Background(), tu, false)

	if stored := getTalosUpgrade(t, r.Client, "test"); len(stored.Status.AlertSilenceIDs) != 0 {
		t.Fatalf("stale silence IDs not cleared: %v", stored.Status.AlertSilenceIDs)
	}
}

func drainEvents(recorder *record.FakeRecorder) []string {
	var events []string
	for {
		select {
		case e := <-recorder.Events:
			events = append(events, e)
		default:
			return events
		}
	}
}

func TestSyncAlertSilences_NoCreateDuringInitialHealthCheck(t *testing.T) {
	tu := newTalosUpgrade("test", withSilences, withPhase(tupprv1alpha1.JobPhaseHealthChecking))
	silencer := &mockSilencer{}
	r := newSilenceReconciler(t, tu, silencer)

	r.syncAlertSilences(context.Background(), tu, false)

	if silencer.ensureCalls != 0 {
		t.Fatalf("expected no silence during the initial health check, got %d Ensure calls", silencer.ensureCalls)
	}
}

func TestSyncAlertSilences_CreatesOnceInFlight(t *testing.T) {
	tu := newTalosUpgrade("test", withSilences, withPhase(tupprv1alpha1.JobPhaseUpgrading))
	silencer := &mockSilencer{returnIDs: []string{testSilenceID}}
	r := newSilenceReconciler(t, tu, silencer)

	r.syncAlertSilences(context.Background(), tu, false)

	if silencer.ensureCalls != 1 || silencer.ensureIDs[0] != "" {
		t.Fatalf("expected one create (empty id), got %d calls with ids %v", silencer.ensureCalls, silencer.ensureIDs)
	}
	if len(silencer.lastMatchers) != 1 || !silencer.lastMatchers[0].IsRegex || !silencer.lastMatchers[0].IsEqual {
		t.Fatalf("matchers not converted: %+v", silencer.lastMatchers)
	}
	stored := getTalosUpgrade(t, r.Client, "test")
	if !slices.Equal(stored.Status.AlertSilenceIDs, []string{testSilenceID}) {
		t.Fatalf("silence IDs not persisted: %v", stored.Status.AlertSilenceIDs)
	}
}

func TestSyncAlertSilences_OneLeasePerEntry(t *testing.T) {
	tu := newTalosUpgrade("test", withSilences, withPhase(tupprv1alpha1.JobPhaseUpgrading))
	tu.Spec.Silences = append(tu.Spec.Silences, tupprv1alpha1.SilenceSpec{
		Matchers: []tupprv1alpha1.SilenceMatcher{
			{Name: "namespace", Value: "rook-ceph", MatchType: "="},
		},
	})
	silencer := &mockSilencer{returnIDs: []string{testSilenceID, testSilenceID2}}
	r := newSilenceReconciler(t, tu, silencer)

	r.syncAlertSilences(context.Background(), tu, false)

	if silencer.ensureCalls != 2 {
		t.Fatalf("expected one Ensure per silences entry, got %d", silencer.ensureCalls)
	}
	stored := getTalosUpgrade(t, r.Client, "test")
	if !slices.Equal(stored.Status.AlertSilenceIDs, []string{testSilenceID, testSilenceID2}) {
		t.Fatalf("silence IDs not persisted per entry: %v", stored.Status.AlertSilenceIDs)
	}

	// Parking the run releases every lease.
	tu.Status.Phase = tupprv1alpha1.JobPhasePending
	r.syncAlertSilences(context.Background(), tu, false)
	if !slices.Equal(silencer.expiredIDs, []string{testSilenceID, testSilenceID2}) {
		t.Fatalf("expected both silences expired, got %v", silencer.expiredIDs)
	}
	if stored := getTalosUpgrade(t, r.Client, "test"); len(stored.Status.AlertSilenceIDs) != 0 || stored.Status.AlertSilencesSince != nil {
		t.Fatalf("silence IDs/since not cleared: %v / %v", stored.Status.AlertSilenceIDs, stored.Status.AlertSilencesSince)
	}
}

func TestSyncAlertSilences_ReplacedSilenceEmitsRecreatedEvent(t *testing.T) {
	// Alertmanager lost the silence (restart, lapse): Ensure mints a new ID
	// and the move is surfaced as an event.
	tu := newTalosUpgrade("test", withSilences,
		withPhase(tupprv1alpha1.JobPhaseUpgrading), withHeldSilence, withSilencesSince(time.Now()))
	silencer := &mockSilencer{returnIDs: []string{testSilenceID2}}
	r := newSilenceReconciler(t, tu, silencer)
	recorder := record.NewFakeRecorder(4)
	r.Recorder = recorder

	r.syncAlertSilences(context.Background(), tu, false)

	if !slices.Equal(tu.Status.AlertSilenceIDs, []string{testSilenceID2}) {
		t.Fatalf("replacement ID not persisted: %v", tu.Status.AlertSilenceIDs)
	}
	if events := drainEvents(recorder); len(events) != 1 || !strings.Contains(events[0], "SilenceRecreated") {
		t.Fatalf("expected a SilenceRecreated event, got %v", events)
	}
}

func TestSyncAlertSilences_ExtendsThroughInterBatchHealthCheck(t *testing.T) {
	// Once held, the silence keeps being extended even while the phase dips
	// back into HealthChecking between batches - no mid-run alert gap.
	tu := newTalosUpgrade("test", withSilences,
		withPhase(tupprv1alpha1.JobPhaseHealthChecking), withHeldSilence)
	silencer := &mockSilencer{}
	r := newSilenceReconciler(t, tu, silencer)

	r.syncAlertSilences(context.Background(), tu, false)

	if silencer.ensureCalls != 1 || silencer.ensureIDs[0] != testSilenceID {
		t.Fatalf("expected extension of %s, got %d calls with ids %v", testSilenceID, silencer.ensureCalls, silencer.ensureIDs)
	}
	if !slices.Equal(tu.Status.AlertSilenceIDs, []string{testSilenceID}) {
		t.Fatalf("silence IDs changed: %v", tu.Status.AlertSilenceIDs)
	}
}

func TestSyncAlertSilences_ExpiresWhenSuspended(t *testing.T) {
	tu := newTalosUpgrade("test", withSilences,
		withPhase(tupprv1alpha1.JobPhaseUpgrading), withHeldSilence)
	silencer := &mockSilencer{}
	r := newSilenceReconciler(t, tu, silencer)

	r.syncAlertSilences(context.Background(), tu, true)

	if silencer.expireCalls != 1 {
		t.Fatalf("expected expire on suspend, got %d calls", silencer.expireCalls)
	}
}

func TestSyncAlertSilences_ExpireFailureStillClearsIDs(t *testing.T) {
	tu := newTalosUpgrade("test", withSilences,
		withPhase(tupprv1alpha1.JobPhaseCompleted), withHeldSilence)
	silencer := &mockSilencer{expireErr: errors.New("am down")}
	r := newSilenceReconciler(t, tu, silencer)

	r.syncAlertSilences(context.Background(), tu, false)

	if stored := getTalosUpgrade(t, r.Client, "test"); len(stored.Status.AlertSilenceIDs) != 0 {
		t.Fatalf("silence IDs should clear even when expire fails (leases lapse): %v", stored.Status.AlertSilenceIDs)
	}
}

func TestSyncAlertSilences_MaxDurationStopsExtending(t *testing.T) {
	tu := newTalosUpgrade("test", withSilences,
		withPhase(tupprv1alpha1.JobPhaseUpgrading), withHeldSilence,
		withSilencesSince(time.Now().Add(-5*time.Hour))) // hold older than the 4h default cap
	silencer := &mockSilencer{}
	r := newSilenceReconciler(t, tu, silencer)
	recorder := record.NewFakeRecorder(4)
	r.Recorder = recorder

	r.syncAlertSilences(context.Background(), tu, false)
	r.syncAlertSilences(context.Background(), tu, false)

	if silencer.ensureCalls != 0 {
		t.Fatalf("expected no extension past maxDuration, got %d Ensure calls", silencer.ensureCalls)
	}
	if !slices.Equal(tu.Status.AlertSilenceIDs, []string{testSilenceID}) {
		t.Fatalf("capped silence ID should stay for later expiry: %v", tu.Status.AlertSilenceIDs)
	}
	// The cap warning is latched: repeated reconciles emit it exactly once.
	if events := drainEvents(recorder); len(events) != 1 || !strings.Contains(events[0], "SilenceMaxDurationReached") {
		t.Fatalf("expected exactly one SilenceMaxDurationReached event, got %v", events)
	}
}

func TestSyncAlertSilences_ParkedRunGetsFreshMaxDurationBudget(t *testing.T) {
	// A run resumed after a long MaintenanceWindow park has no hold (IDs and
	// alertSilencesSince were cleared at park), so silences are re-created
	// regardless of how long ago the RUN started.
	tu := newTalosUpgrade("test", withSilences, withPhase(tupprv1alpha1.JobPhaseUpgrading))
	started := metav1.NewTime(time.Now().Add(-9 * time.Hour)) // run started long ago
	tu.Status.StartedAt = &started
	silencer := &mockSilencer{returnIDs: []string{testSilenceID}}
	r := newSilenceReconciler(t, tu, silencer)

	r.syncAlertSilences(context.Background(), tu, false)

	if silencer.ensureCalls != 1 {
		t.Fatalf("expected the resumed run to re-create its silence, got %d Ensure calls", silencer.ensureCalls)
	}
	stored := getTalosUpgrade(t, r.Client, "test")
	if stored.Status.AlertSilencesSince == nil {
		t.Fatal("expected alertSilencesSince to start a fresh hold budget")
	}
}

func TestSyncAlertSilences_EnsureErrorNeverGatesTheRun(t *testing.T) {
	tu := newTalosUpgrade("test", withSilences, withPhase(tupprv1alpha1.JobPhaseUpgrading))
	silencer := &mockSilencer{ensureErr: errors.New("am down")}
	r := newSilenceReconciler(t, tu, silencer)

	r.syncAlertSilences(context.Background(), tu, false)

	if stored := getTalosUpgrade(t, r.Client, "test"); len(stored.Status.AlertSilenceIDs) != 0 {
		t.Fatalf("no silence IDs should persist on Ensure failure: %v", stored.Status.AlertSilenceIDs)
	}
}

func TestSyncAlertSilences_NoSilencesSpecExpiresLeftover(t *testing.T) {
	// silences removed from the spec mid-run: the held silence is released.
	tu := newTalosUpgrade("test", withPhase(tupprv1alpha1.JobPhaseUpgrading), withHeldSilence)
	silencer := &mockSilencer{}
	r := newSilenceReconciler(t, tu, silencer)

	r.syncAlertSilences(context.Background(), tu, false)

	if silencer.expireCalls != 1 {
		t.Fatalf("expected leftover silence expired, got %d calls", silencer.expireCalls)
	}
}
