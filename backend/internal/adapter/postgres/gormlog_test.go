package postgres

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func newTestLogger(level slog.Level) (*gormLogger, *bytes.Buffer) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: level}))
	return newGormLogger(log, 100*time.Millisecond), &buf
}

func query() (string, int64) { return `SELECT * FROM "orders" WHERE id = $1`, 1 }

func TestGormLogger_Trace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		level   slog.Level
		elapsed time.Duration
		err     error
		want    string // empty means nothing logged
	}{
		{name: "error", level: slog.LevelInfo, err: errors.New("deadlock"), want: `"msg":"query failed"`},
		{name: "not found is not an error", level: slog.LevelInfo, err: gorm.ErrRecordNotFound},
		{name: "slow", level: slog.LevelInfo, elapsed: 200 * time.Millisecond, want: `"msg":"slow query"`},
		{name: "fast at info", level: slog.LevelInfo},
		{name: "fast at debug", level: slog.LevelDebug, want: `"msg":"query"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l, buf := newTestLogger(tt.level)
			l.Trace(context.Background(), time.Now().Add(-tt.elapsed), query, tt.err)

			got := buf.String()
			if tt.want == "" {
				if got != "" {
					t.Errorf("logged %s, want nothing", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) || !strings.Contains(got, `"component":"gorm"`) {
				t.Errorf("log = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestGormLogger_ParamsFilterDropsValues(t *testing.T) {
	t.Parallel()

	l, _ := newTestLogger(slog.LevelInfo)
	sql, params := l.ParamsFilter(context.Background(), "SELECT $1", "Maria da Silva")

	if sql != "SELECT $1" || params != nil {
		t.Errorf("ParamsFilter = %q, %v; want the SQL unchanged and no params", sql, params)
	}
}

func TestGormLogger_Messages(t *testing.T) {
	t.Parallel()

	l, buf := newTestLogger(slog.LevelInfo)
	if l.LogMode(0) != l {
		t.Error("LogMode must return the same logger")
	}
	l.Info(context.Background(), "info %d", 1)
	l.Warn(context.Background(), "warn %d", 2)
	l.Error(context.Background(), "error %d", 3)

	for _, want := range []string{"info 1", "warn 2", "error 3"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %q in %s", want, buf.String())
		}
	}
}
