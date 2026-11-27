package config

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestLoad_Storage(t *testing.T) {
	t.Parallel()

	e := minimalEnv()
	e["STORAGE_PUBLIC_ENDPOINT"] = "localhost:9000"
	e["STORAGE_USE_SSL"] = "false"
	cfg, err := Load(env(e))
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.Storage
	if s.Endpoint != "minio:9000" || s.PublicEndpoint != "localhost:9000" || s.Bucket != "encomendo-photos" ||
		s.Region != "us-east-1" || s.AccessKey != "encomendo" || s.SecretKey.Value() != "a-local-secret" || s.UseSSL {
		t.Errorf("Storage = %+v", s)
	}
}

func TestLoad_StorageDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load(env(productionEnv()))
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.Storage
	if s.PublicEndpoint != s.Endpoint || !s.UseSSL || s.Region != "auto" {
		t.Errorf("Storage = %+v, want the public endpoint to default to the endpoint, and SSL on", s)
	}
}

func TestLoad_StorageRefusals(t *testing.T) {
	t.Parallel()

	tests := map[string]struct{ key, value, want string }{
		"no endpoint":            {"STORAGE_ENDPOINT", "", "STORAGE_ENDPOINT is required"},
		"no bucket":              {"STORAGE_BUCKET", "", "STORAGE_BUCKET is required"},
		"no access key":          {"STORAGE_ACCESS_KEY", "", "STORAGE_ACCESS_KEY is required"},
		"no secret key":          {"STORAGE_SECRET_KEY", "", "STORAGE_SECRET_KEY is required"},
		"an endpoint URL":        {"STORAGE_ENDPOINT", "http://minio:9000", "host[:port]"},
		"a public endpoint path": {"STORAGE_PUBLIC_ENDPOINT", "localhost:9000/bucket", "host[:port]"},
		"credentials in a host":  {"STORAGE_ENDPOINT", "key:secret@minio:9000", "host[:port]"},
		"a bad SSL flag":         {"STORAGE_USE_SSL", "sometimes", "STORAGE_USE_SSL"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := minimalEnv()
			e[tt.key] = tt.value
			if _, err := Load(env(e)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Load error = %v, want one about %s", err, tt.want)
			}
		})
	}
}

// The secret key never reaches a log or an error message, whatever prints
// the configuration.
func TestStorage_SecretIsNeverPrinted(t *testing.T) {
	t.Parallel()

	cfg, err := Load(env(minimalEnv()))
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	log.Info("config", "storage", cfg.Storage, "secret", cfg.Storage.SecretKey)
	slog.New(slog.NewTextHandler(&logs, nil)).Info("config", "secret", cfg.Storage.SecretKey)
	printed := fmt.Sprintf("%v %+v %#v %s %v", cfg, cfg.Storage, cfg.Storage, cfg.Storage.SecretKey, cfg.Storage.SecretKey) + logs.String()
	if strings.Contains(printed, "a-local-secret") {
		t.Errorf("the secret was printed: %s", printed)
	}
	if !strings.Contains(printed, "[redacted]") {
		t.Errorf("no redaction mark in %s", printed)
	}
	if (Secret{}).String() != "[unset]" {
		t.Errorf("unset secret = %q", Secret{}.String())
	}
}
