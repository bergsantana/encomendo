package config

import (
	"log/slog"
	"strings"
)

// Secret is a setting that must never be printed or logged: every format
// verb and slog show "[redacted]". Value returns the setting itself, for
// the one client that needs it.
type Secret struct{ value string }

// Value returns the secret, to hand to the client that uses it.
func (s Secret) Value() string { return s.value }

// String implements fmt.Stringer without the secret.
func (s Secret) String() string { return redacted(s.value) }

// GoString implements fmt.GoStringer, for %#v, without the secret.
func (s Secret) GoString() string { return redacted(s.value) }

// LogValue implements slog.LogValuer without the secret.
func (s Secret) LogValue() slog.Value { return slog.StringValue(redacted(s.value)) }

func redacted(v string) string {
	if v == "" {
		return "[unset]"
	}
	return "[redacted]"
}

// Storage configures the photo bucket on any S3-compatible store: MinIO in
// development, CI and tests, Cloudflare R2 in production (ADR 0024).
type Storage struct {
	// Endpoint is host[:port] the API itself reaches the store at, for
	// reads, copies and deletes: minio:9000 inside compose, or
	// {account}.r2.cloudflarestorage.com.
	Endpoint string
	// PublicEndpoint is host[:port] signed URLs name, so browsers and the
	// Android emulator (through adb reverse tcp:9000) reach it: localhost:9000
	// locally (ADR 0025). Signing binds the host, so it cannot be
	// rewritten later. It is Endpoint when unset.
	PublicEndpoint string
	Bucket         string
	// Region is part of every signature: us-east-1 for MinIO, auto for R2.
	Region    string
	AccessKey string
	SecretKey Secret
	// UseSSL turns on HTTPS to both endpoints; off only for a local MinIO.
	UseSSL bool
}

func (r *reader) storage(env Environment) Storage {
	s := Storage{
		Endpoint:  r.required("STORAGE_ENDPOINT"),
		Bucket:    r.required("STORAGE_BUCKET"),
		Region:    r.str("STORAGE_REGION", "us-east-1"),
		AccessKey: r.required("STORAGE_ACCESS_KEY"),
		SecretKey: Secret{value: r.required("STORAGE_SECRET_KEY")},
		UseSSL:    r.boolean("STORAGE_USE_SSL", true),
	}
	s.PublicEndpoint = r.str("STORAGE_PUBLIC_ENDPOINT", s.Endpoint)
	for key, host := range map[string]string{"STORAGE_ENDPOINT": s.Endpoint, "STORAGE_PUBLIC_ENDPOINT": s.PublicEndpoint} {
		r.check(host == "" || !strings.ContainsAny(host, "/?#@ "),
			"%s must be host[:port], without a scheme or a path, got %q", key, host)
	}
	r.check(env.IsLocal() || s.UseSSL, "STORAGE_USE_SSL=false is for a local MinIO only (ADR 0024)")
	return s
}
