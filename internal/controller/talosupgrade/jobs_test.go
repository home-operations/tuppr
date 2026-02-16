package talosupgrade

import (
	"context"
	"fmt"
	"net"
	"syscall"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
			err:  &net.OpError{Op: "dial", Err: &net.DNSError{Err: "timeout", IsTimeout: true}},
			want: true,
		},
		{
			name: "network timeout with timeout=true is transient",
			err:  &net.OpError{Op: "read", Err: &net.DNSError{Err: "timeout", IsTimeout: true}},
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
