package main

import (
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
