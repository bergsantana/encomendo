package storage

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

const (
	testKey    = "t/0192f3a4-b5c6-7d8e-9fab-cdef01234567/items/0192f3a4-b5c6-7d8e-afab-cdef01234568/0192f3a4-b5c6-7d8e-bfab-cdef01234569"
	testSecret = "a-very-secret-key-0123456789"
)

// fakeS3 answers every request with status, and records what it was asked.
type fakeS3 struct {
	status   int
	requests []*http.Request
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.requests = append(f.requests, r.Clone(context.Background()))
	switch f.status {
	case http.StatusOK:
		w.Header().Set("ETag", `"abc"`)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		switch r.Method {
		case http.MethodPut:
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<CopyObjectResult><ETag>"abc"</ETag><LastModified>2026-11-27T12:00:00.000Z</LastModified></CopyObjectResult>`))
		default:
			w.Header().Set("Content-Length", "3")
			w.WriteHeader(http.StatusOK)
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte{0xFF, 0xD8, 0xFF})
			}
		}
	default:
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(f.status)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte(`<?xml version="1.0"?><Error><Code>` + code(f.status) + `</Code><Message>m</Message></Error>`))
		}
	}
}

func code(status int) string {
	switch status {
	case http.StatusNotFound:
		return "NoSuchKey"
	case http.StatusPreconditionFailed:
		return "PreconditionFailed"
	default:
		return "InternalError"
	}
}

func newTestStore(t *testing.T, status int) (*S3, *fakeS3, *bytes.Buffer) {
	t.Helper()
	f := &fakeS3{status: status}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	var logs bytes.Buffer
	s, err := New(Config{
		Endpoint: strings.TrimPrefix(srv.URL, "http://"), PublicEndpoint: "localhost:9000",
		Bucket: "encomendo-photos", Region: "us-east-1", AccessKey: "encomendo", SecretKey: testSecret,
	}, slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return s, f, &logs
}

func TestS3_SignsForThePublicEndpoint(t *testing.T) {
	t.Parallel()
	s, f, _ := newTestStore(t, http.StatusOK)
	up, err := s.PresignPut(t.Context(), testKey, "image/png", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(up.URL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "localhost:9000" || u.Path != "/encomendo-photos/"+UploadPrefix+testKey {
		t.Errorf("upload URL %s://%s%s, want the staging key on the public endpoint", u.Scheme, u.Host, u.Path)
	}
	q := u.Query()
	if q.Get("X-Amz-Expires") != "300" || !strings.Contains(q.Get("X-Amz-SignedHeaders"), "content-type") {
		t.Errorf("signed for %s s with headers %q; want 300 s and content-type", q.Get("X-Amz-Expires"), q.Get("X-Amz-SignedHeaders"))
	}
	if up.Method != http.MethodPut || len(up.Headers) != 1 || up.Headers["Content-Type"] != "image/png" {
		t.Errorf("upload = %+v", up)
	}
	if d := time.Until(up.ExpiresAt); d < 4*time.Minute || d > 5*time.Minute {
		t.Errorf("expiresAt in %v", d)
	}
	get, expires, err := s.SignGet(t.Context(), testKey, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	g, err := url.Parse(get)
	if err != nil {
		t.Fatal(err)
	}
	if g.Host != "localhost:9000" || g.Path != "/encomendo-photos/"+testKey || g.Query().Get("X-Amz-Expires") != "600" {
		t.Errorf("read URL = %s", g.Redacted())
	}
	if d := time.Until(expires); d < 9*time.Minute || d > 10*time.Minute {
		t.Errorf("read expires in %v", d)
	}
	if strings.Contains(up.URL+get, testSecret) {
		t.Error("a signed URL holds the secret key")
	}
	if len(f.requests) != 0 {
		t.Errorf("signing made %d requests, want none", len(f.requests))
	}
}

func TestS3_MapsTheStoresAnswers(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		status int
		want   error
	}{
		"missing":  {http.StatusNotFound, appcatalog.ErrObjectMissing},
		"replaced": {http.StatusPreconditionFailed, appcatalog.ErrObjectChanged},
		"down":     {http.StatusInternalServerError, shared.ErrUnavailable},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, _, _ := newTestStore(t, tt.status)
			ctx := t.Context()
			if tt.status != http.StatusPreconditionFailed {
				if _, err := s.Stat(ctx, testKey); !errors.Is(err, tt.want) {
					t.Errorf("Stat = %v, want %v", err, tt.want)
				}
			}
			if _, err := s.Head(ctx, testKey, "abc", 512); !errors.Is(err, tt.want) {
				t.Errorf("Head = %v, want %v", err, tt.want)
			}
			if err := s.Promote(ctx, testKey, "abc", "image/jpeg"); !errors.Is(err, tt.want) {
				t.Errorf("Promote = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestS3_StatReadsTheStagedUpload(t *testing.T) {
	t.Parallel()
	s, f, _ := newTestStore(t, http.StatusOK)
	info, err := s.Stat(t.Context(), testKey)
	if err != nil || info.Size != 3 || info.ETag != "abc" {
		t.Fatalf("Stat = %+v, %v", info, err)
	}
	head, err := s.Head(t.Context(), testKey, info.ETag, 512)
	if err != nil || !bytes.Equal(head, []byte{0xFF, 0xD8, 0xFF}) {
		t.Fatalf("Head = %v, %v", head, err)
	}
	if err := s.Promote(t.Context(), testKey, info.ETag, "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	stat, get, put := f.requests[0], f.requests[1], f.requests[2]
	if stat.URL.Path != "/encomendo-photos/"+UploadPrefix+testKey {
		t.Errorf("Stat read %s, want the staged upload", stat.URL.Path)
	}
	if get.Header.Get("Range") != "bytes=0-511" || get.Header.Get("If-Match") != `"abc"` {
		t.Errorf("Head sent Range %q and If-Match %q", get.Header.Get("Range"), get.Header.Get("If-Match"))
	}
	if put.URL.Path != "/encomendo-photos/"+testKey || put.Header.Get("X-Amz-Copy-Source-If-Match") != `"abc"` ||
		put.Header.Get("X-Amz-Metadata-Directive") != "REPLACE" || put.Header.Get("Content-Type") != "image/jpeg" {
		t.Errorf("Promote sent %s %s with headers %v", put.Method, put.URL.Path, put.Header)
	}
	if !strings.HasSuffix(put.Header.Get("X-Amz-Copy-Source"), UploadPrefix+testKey) {
		t.Errorf("Promote copied from %q", put.Header.Get("X-Amz-Copy-Source"))
	}
}

func TestS3_DeleteFailuresAreLoggedWithoutSecrets(t *testing.T) {
	t.Parallel()
	s, f, logs := newTestStore(t, http.StatusInternalServerError)
	if err := s.Delete(t.Context(), testKey); !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("Delete = %v", err)
	}
	if err := s.DeleteUpload(t.Context(), testKey); !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("DeleteUpload = %v", err)
	}
	paths := map[string]bool{}
	for _, r := range f.requests {
		paths[r.Method+" "+r.URL.Path] = true
	}
	if !paths["DELETE /encomendo-photos/"+testKey] || !paths["DELETE /encomendo-photos/"+UploadPrefix+testKey] || len(paths) != 2 {
		t.Errorf("requests = %v, want a delete of the photo and of the upload", paths)
	}
	out := logs.String()
	if !strings.Contains(out, testKey) || strings.Count(out, "level=") != 2 {
		t.Errorf("logs = %s, want both failures with the key", out)
	}
	if strings.Contains(out, testSecret) || strings.Contains(out, "X-Amz-Signature") {
		t.Errorf("logs hold a secret: %s", out)
	}
}

func TestS3_HeadRefusesAMisuse(t *testing.T) {
	t.Parallel()
	s, f, _ := newTestStore(t, http.StatusOK)
	for _, call := range []func() error{
		func() error { _, err := s.Head(t.Context(), testKey, "", 512); return err },
		func() error { _, err := s.Head(t.Context(), testKey, "abc", 0); return err },
	} {
		err := call()
		if err == nil || errors.Is(err, shared.ErrInvalid) || errors.Is(err, shared.ErrUnavailable) {
			t.Errorf("misuse = %v, want a bug error", err)
		}
	}
	if len(f.requests) != 0 {
		t.Errorf("a misuse reached the store")
	}
}

func TestNew_RefusesAMalformedEndpoint(t *testing.T) {
	t.Parallel()
	_, err := New(Config{Endpoint: "http://minio:9000/x", PublicEndpoint: "localhost:9000", Bucket: "b", Region: "us-east-1", SecretKey: testSecret}, slog.Default())
	if err == nil || strings.Contains(err.Error(), testSecret) {
		t.Errorf("New = %v", err)
	}
}
