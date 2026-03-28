package logutil

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// Options controls the common logger setup used by CLI tools in this repo.
type Options struct {
	Level     slog.Leveler
	AddSource bool
}

// New returns a text logger backed by the provided writer.
func New(w io.Writer, opts Options) *slog.Logger {
	level := opts.Level
	if level == nil {
		level = slog.LevelInfo
	}

	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level:     level,
		AddSource: opts.AddSource,
	}))
}

// ParseLevel parses a textual log level for CLI configuration.
func ParseLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", raw)
	}
}
