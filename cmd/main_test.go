package main

import (
	"strings"
	"testing"

	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestApplyLogLevel(t *testing.T) {
	tests := []struct {
		name        string
		level       string
		wantErr     bool
		wantDevel   bool
		wantEnabled zapcore.Level
		wantMuted   zapcore.Level
	}{
		{name: "debug", level: "debug", wantDevel: true, wantEnabled: zapcore.DebugLevel, wantMuted: zapcore.DebugLevel - 1},
		{name: "info", level: "info", wantEnabled: zapcore.InfoLevel, wantMuted: zapcore.DebugLevel},
		{name: "warn", level: "warn", wantEnabled: zapcore.WarnLevel, wantMuted: zapcore.InfoLevel},
		{name: "error", level: "error", wantEnabled: zapcore.ErrorLevel, wantMuted: zapcore.WarnLevel},
		{name: "case insensitive", level: "WARN", wantEnabled: zapcore.WarnLevel, wantMuted: zapcore.InfoLevel},
		{name: "unknown", level: "trace", wantErr: true},
		{name: "empty", level: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts zap.Options
			err := applyLogLevel(&opts, tt.level)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("applyLogLevel(%q) = nil, want error", tt.level)
				}
				return
			}
			if err != nil {
				t.Fatalf("applyLogLevel(%q) = %v", tt.level, err)
			}
			if opts.Development != tt.wantDevel {
				t.Errorf("Development = %v, want %v", opts.Development, tt.wantDevel)
			}
			if !opts.Level.Enabled(tt.wantEnabled) {
				t.Errorf("level %v should be enabled", tt.wantEnabled)
			}
			if opts.Level.Enabled(tt.wantMuted) {
				t.Errorf("level %v should be muted", tt.wantMuted)
			}
		})
	}
}

func TestApplyLogFormat(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		wantErr  bool
		wantJSON bool
	}{
		{name: "logfmt", format: "logfmt"},
		{name: "json", format: "json", wantJSON: true},
		{name: "case insensitive", format: "JSON", wantJSON: true},
		{name: "unknown", format: "console", wantErr: true},
		{name: "empty", format: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts zap.Options
			err := applyLogFormat(&opts, tt.format)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("applyLogFormat(%q) = nil, want error", tt.format)
				}
				return
			}
			if err != nil {
				t.Fatalf("applyLogFormat(%q) = %v", tt.format, err)
			}
			buf, err := opts.Encoder.EncodeEntry(zapcore.Entry{Level: zapcore.InfoLevel, Message: "hello"}, nil)
			if err != nil {
				t.Fatalf("EncodeEntry: %v", err)
			}
			line := buf.String()
			if got := strings.HasPrefix(line, "{"); got != tt.wantJSON {
				t.Fatalf("json output = %v, want %v: %q", got, tt.wantJSON, line)
			}
		})
	}
}
