package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// gormLogger sends GORM's logs to slog. Queries are logged with placeholders
// ($1, $2 …) and never with their values, because values can be customer
// names, phones and other personal data.
type gormLogger struct {
	log  *slog.Logger
	slow time.Duration
}

var (
	_ gormlogger.Interface = (*gormLogger)(nil)
	_ gorm.ParamsFilter    = (*gormLogger)(nil)
)

func newGormLogger(log *slog.Logger, slow time.Duration) *gormLogger {
	return &gormLogger{log: log.With("component", "gorm"), slow: slow}
}

// LogMode is part of GORM's interface. The level is set by slog instead.
func (l *gormLogger) LogMode(gormlogger.LogLevel) gormlogger.Interface { return l }

// Info logs a GORM informational message.
func (l *gormLogger) Info(ctx context.Context, msg string, args ...any) {
	l.log.InfoContext(ctx, fmt.Sprintf(msg, args...))
}

// Warn logs a GORM warning.
func (l *gormLogger) Warn(ctx context.Context, msg string, args ...any) {
	l.log.WarnContext(ctx, fmt.Sprintf(msg, args...))
}

// Error logs a GORM error message.
func (l *gormLogger) Error(ctx context.Context, msg string, args ...any) {
	l.log.ErrorContext(ctx, fmt.Sprintf(msg, args...))
}

// ParamsFilter drops query values, so GORM explains SQL with placeholders.
func (l *gormLogger) ParamsFilter(_ context.Context, sql string, _ ...any) (string, []any) {
	return sql, nil
}

// Trace logs each query: errors at error level, slow queries at warn level,
// everything else at debug. Not-found is a normal result, not an error.
func (l *gormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	elapsed := time.Since(begin)
	switch {
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
		sql, rows := fc()
		l.log.ErrorContext(ctx, "query failed", "sql", sql, "rows", rows, "duration", elapsed, "err", err)
	case l.slow > 0 && elapsed > l.slow:
		sql, rows := fc()
		l.log.WarnContext(ctx, "slow query", "sql", sql, "rows", rows, "duration", elapsed)
	case l.log.Enabled(ctx, slog.LevelDebug):
		sql, rows := fc()
		l.log.DebugContext(ctx, "query", "sql", sql, "rows", rows, "duration", elapsed)
	}
}
