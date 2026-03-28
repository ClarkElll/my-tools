package logutil

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "default", input: "", want: "INFO"},
		{name: "debug", input: "debug", want: "DEBUG"},
		{name: "warn", input: "warn", want: "WARN"},
		{name: "warning", input: "warning", want: "WARN"},
		{name: "error", input: "error", want: "ERROR"},
		{name: "invalid", input: "trace", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, err := ParseLevel(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.input)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseLevel returned error: %v", err)
			}

			if got := level.String(); got != tt.want {
				t.Fatalf("expected level %q, got %q", tt.want, got)
			}
		})
	}
}

func TestNewDefaultsToInfoLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, Options{})

	logger.Debug("debug message")
	logger.Info("info message", "tool", "remote-write-bench")

	output := buf.String()
	if strings.Contains(output, "debug message") {
		t.Fatalf("debug log should not be emitted by default: %q", output)
	}
	if !strings.Contains(output, "info message") {
		t.Fatalf("expected info log in output, got %q", output)
	}
	if !strings.Contains(output, "tool=remote-write-bench") {
		t.Fatalf("expected structured field in output, got %q", output)
	}
}
