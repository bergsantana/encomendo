package logger

import (
	"io"
	"log/slog"

	"github.com/bergsantana/encomendo/backend/internal/platform/config"
)

// New returns a slog.Logger writing to w in the configured format and level.
// The service attribute lets one log stream hold api, migrate and worker lines.
func New(w io.Writer, cfg config.Log, service string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(cfg.Level)}

	var h slog.Handler
	if cfg.Format == "text" {
		h = slog.NewTextHandler(w, opts)
	} else {
		h = slog.NewJSONHandler(w, opts)
	}
	return slog.New(h).With(slog.String("service", service))
}

// parseLevel maps a validated config level to slog. Config validation rejects
// anything else, so the default branch only guards against future changes.
func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
