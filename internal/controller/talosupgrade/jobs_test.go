package talosupgrade

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"

	"github.com/home-operations/tuppr/internal/constants"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testTimeoutErr = "timeout"
	testV110Talos  = "v1.10.0"
	testV121       = "v1.12.1"
)

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error returns false",
			err:  nil,
			want: false,
		},
		{
			name: "context deadline exceeded is transient",
			err:  context.DeadlineExceeded,
			want: true,
		},
		{
			name: "gRPC Unavailable is transient",
			err:  status.Error(codes.Unavailable, "connection refused"),
			want: true,
		},
		{
			name: "gRPC ResourceExhausted is transient",
			err:  status.Error(codes.ResourceExhausted, "rate limit exceeded"),
			want: true,
		},
		{
			name: "gRPC DeadlineExceeded is transient",
			err:  status.Error(codes.DeadlineExceeded, "deadline exceeded"),
			want: true,
		},
		{
			name: "gRPC Unauthenticated is permanent",
			err:  status.Error(codes.Unauthenticated, "invalid credentials"),
			want: false,
		},
		{
			name: "gRPC NotFound is permanent",
			err:  status.Error(codes.NotFound, "resource not found"),
			want: false,
		},
		{
			name: "wrapped gRPC Unavailable is transient",
			err:  fmt.Errorf("operation failed: %w", status.Error(codes.Unavailable, "node rebooting")),
			want: true,
		},
		{
			name: "network timeout error is transient",
			err:  &net.OpError{Op: "dial", Err: &net.DNSError{Err: testTimeoutErr, IsTimeout: true}},
			want: true,
		},
		{
			name: "network timeout with timeout=true is transient",
			err:  &net.OpError{Op: "read", Err: &net.DNSError{Err: testTimeoutErr, IsTimeout: true}},
			want: true,
		},
		{
			name: "ECONNREFUSED is transient",
			err:  syscall.ECONNREFUSED,
			want: true,
		},
		{
			name: "ECONNRESET is transient",
			err:  syscall.ECONNRESET,
			want: true,
		},
		{
			name: "ETIMEDOUT is transient",
			err:  syscall.ETIMEDOUT,
			want: true,
		},
		{
			name: "EPIPE is transient",
			err:  syscall.EPIPE,
			want: true,
		},
		{
			name: "error string containing connection refused is transient",
			err:  fmt.Errorf("failed to connect: connection refused"),
			want: true,
		},
		{
			name: "error string containing connection reset is transient",
			err:  fmt.Errorf("connection reset by peer"),
			want: true,
		},
		{
			name: "error string containing i/o timeout is transient",
			err:  fmt.Errorf("read failed: i/o timeout"),
			want: true,
		},
		{
			name: "error string containing EOF is transient",
			err:  fmt.Errorf("unexpected EOF"),
			want: true,
		},
		{
			name: "permanent error - generic message",
			err:  fmt.Errorf("something went wrong"),
			want: false,
		},
		{
			name: "permanent error - permission denied",
			err:  fmt.Errorf("permission denied"),
			want: false,
		},
		{
			name: "permanent error - not found",
			err:  fmt.Errorf("resource not found"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTransientError(tt.err)
			if got != tt.want {
				t.Errorf("isTransientError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsNodeReady(t *testing.T) {
	tests := []struct {
		name string
		node *corev1.Node
		want bool
	}{
		{
			name: "node with Ready=True is ready",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					},
				},
			},
			want: true,
		},
		{
			name: "node with Ready=False is not ready",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
					},
				},
			},
			want: false,
		},
		{
			name: "node with Ready=Unknown is not ready",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{Type: corev1.NodeReady, Status: corev1.ConditionUnknown},
					},
				},
			},
			want: false,
		},
		{
			name: "node with no conditions is not ready",
			node: &corev1.Node{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNodeReady(tt.node)
			if got != tt.want {
				t.Errorf("isNodeReady() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCreateJob_SendsNotification(t *testing.T) {
	scheme := newTestScheme()
	tu := newTalosUpgrade("test-upgrade", withFinalizer)
	node := newNode(fakeNodeA, testNodeIP1)

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node).
		Build()

	r := newTalosReconciler(cl, scheme, &mockTalosClient{
		nodeVersions: map[string]string{testNodeIP1: testV110Talos},
	}, &mockHealthChecker{})
	notifier := &mockNotifier{}
	r.Notifier = notifier

	targetImage := "factory.talos.dev/installer/abc:" + fakeTalosVersion

	job, err := r.createJob(context.Background(), tu, fakeNodeA, targetImage)
	if err != nil {
		t.Fatalf("createJob returned unexpected error: %v", err)
	}
	if job == nil {
		t.Fatal("expected createJob to return a job")
	}

	if notifier.calls != 1 {
		t.Fatalf("expected one notification for the newly created job, got %d", notifier.calls)
	}
	if notifier.lastTitle != "Tuppr Upgrade Started" {
		t.Fatalf("expected start notification title, got %q", notifier.lastTitle)
	}
	if notifier.lastMessage != "Node "+fakeNodeA+" is upgrading Talos from v1.10.0 -> "+fakeTalosVersion {
		t.Fatalf("expected start notification message for %s, got %q", fakeNodeA, notifier.lastMessage)
	}
}

func TestCreateJob_SendsNotification_WithVersionOverride(t *testing.T) {
	scheme := newTestScheme()
	tu := newTalosUpgrade("test-upgrade", withFinalizer)
	node := newNode(fakeNodeA, testNodeIP1)
	node.Annotations = map[string]string{
		constants.VersionAnnotation: testV121,
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node).
		Build()

	r := newTalosReconciler(cl, scheme, &mockTalosClient{
		nodeVersions: map[string]string{testNodeIP1: testV110Talos},
	}, &mockHealthChecker{})
	notifier := &mockNotifier{}
	r.Notifier = notifier

	targetImage := "factory.talos.dev/installer/abc:v1.12.1"

	job, err := r.createJob(context.Background(), tu, fakeNodeA, targetImage)
	if err != nil {
		t.Fatalf("createJob returned unexpected error: %v", err)
	}
	if job == nil {
		t.Fatal("expected createJob to return a job")
	}

	expected := "Node " + fakeNodeA + " is upgrading Talos from v1.10.0 -> v1.12.1"
	if notifier.lastMessage != expected {
		t.Fatalf("expected override notification message %q, got %q", expected, notifier.lastMessage)
	}
}

func TestCreateJob_NotifierErrorDoesNotFailJobCreation(t *testing.T) {
	scheme := newTestScheme()
	tu := newTalosUpgrade("test-upgrade", withFinalizer)
	node := newNode(fakeNodeA, testNodeIP1)

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node).
		Build()

	r := newTalosReconciler(cl, scheme, &mockTalosClient{}, &mockHealthChecker{})
	notifier := &mockNotifier{sendErr: errors.New("boom")}
	r.Notifier = notifier

	targetImage := "factory.talos.dev/installer/abc:" + fakeTalosVersion

	job, err := r.createJob(context.Background(), tu, fakeNodeA, targetImage)
	if err != nil {
		t.Fatalf("expected createJob to ignore notifier errors, got: %v", err)
	}
	if job == nil {
		t.Fatal("expected createJob to return a job")
	}
	if notifier.calls != 1 {
		t.Fatalf("expected one notification attempt, got %d", notifier.calls)
	}
}

func TestCreateJob_SendsFallbackMessage_WhenCurrentVersionLookupFails(t *testing.T) {
	scheme := newTestScheme()
	tu := newTalosUpgrade("test-upgrade", withFinalizer)
	node := newNode(fakeNodeA, testNodeIP1)

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(tu, node).
		Build()

	r := newTalosReconciler(cl, scheme, &mockTalosClient{
		getVersionErr: errors.New("boom"),
	}, &mockHealthChecker{})
	notifier := &mockNotifier{}
	r.Notifier = notifier

	targetImage := "factory.talos.dev/installer/abc:" + fakeTalosVersion

	job, err := r.createJob(context.Background(), tu, fakeNodeA, targetImage)
	if err != nil {
		t.Fatalf("expected createJob to ignore current version lookup errors, got: %v", err)
	}
	if job == nil {
		t.Fatal("expected createJob to return a job")
	}

	expected := "Starting upgrade for node " + fakeNodeA
	if notifier.lastMessage != expected {
		t.Fatalf("expected fallback notification message %q, got %q", expected, notifier.lastMessage)
	}
}
