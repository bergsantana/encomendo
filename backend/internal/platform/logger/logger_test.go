package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/platform/config"
)

func TestNew_WritesJSONWithService(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	log := New(&buf, config.Log{Level: "info", Format: "json"}, "api")
	log.Info("started", "addr", ":8080")

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log line is not JSON: %v (%q)", err, buf.String())
	}
	if line["service"] != "api" || line["msg"] != "started" || line["addr"] != ":8080" {
		t.Errorf("unexpected log line: %v", line)
	}
}

func TestNew_RespectsLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		level     string
		debugSeen bool
		infoSeen  bool
		warnSeen  bool
	}{
		{level: "debug", debugSeen: true, infoSeen: true, warnSeen: true},
		{level: "info", debugSeen: false, infoSeen: true, warnSeen: true},
		{level: "warn", debugSeen: false, infoSeen: false, warnSeen: true},
		{level: "error", debugSeen: false, infoSeen: false, warnSeen: false},
	}
	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			log := New(&buf, config.Log{Level: tt.level, Format: "text"}, "api")
			log.Debug("debug-line")
			log.Info("info-line")
			log.Warn("warn-line")

			out := buf.String()
			if got := strings.Contains(out, "debug-line"); got != tt.debugSeen {
				t.Errorf("debug line present = %v, want %v", got, tt.debugSeen)
			}
			if got := strings.Contains(out, "info-line"); got != tt.infoSeen {
				t.Errorf("info line present = %v, want %v", got, tt.infoSeen)
			}
			if got := strings.Contains(out, "warn-line"); got != tt.warnSeen {
				t.Errorf("warn line present = %v, want %v", got, tt.warnSeen)
			}
		})
	}
}
