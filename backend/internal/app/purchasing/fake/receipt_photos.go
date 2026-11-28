package fake

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
)

// object is one stored object: its bytes, content type and version.
type object struct {
	data        []byte
	contentType string
	etag        string
}

// ReceiptPhotos is an in-memory apppurchasing.ReceiptPhotos with the two
// places of the real one: staged uploads, and promoted photos. Promoting
// copies, so the staged upload stays until the bucket's lifecycle removes
// it. Signed URLs are "https://uploads.test/{key}", valid until the clock's
// now plus the TTL. It logs every call, so a test can check that a refused
// key never reached storage.
type ReceiptPhotos struct {
	mu      sync.Mutex
	clock   appshared.Clock
	uploads map[string]object
	photos  map[string]object
	calls   []string
	faults  map[string]error
	before  map[string]func()
	nextTag int
	orphans []string
}

var _ apppurchasing.ReceiptPhotos = (*ReceiptPhotos)(nil)

// NewReceiptPhotos returns an empty store on clock.
func NewReceiptPhotos(clock appshared.Clock) *ReceiptPhotos {
	return &ReceiptPhotos{
		clock: clock, uploads: map[string]object{}, photos: map[string]object{},
		faults: map[string]error{}, before: map[string]func(){},
	}
}

// Upload stages data at key, as an app's signed PUT would, with a new
// version each time.
func (s *ReceiptPhotos) Upload(key, contentType string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextTag++
	s.uploads[key] = object{data: append([]byte(nil), data...), contentType: contentType, etag: "v" + strconv.Itoa(s.nextTag)}
}

// Uploaded reports whether an upload is staged at key.
func (s *ReceiptPhotos) Uploaded(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.uploads[key]
	return ok
}

// Promoted returns the photo object at key and its content type.
func (s *ReceiptPhotos) Promoted(key string) ([]byte, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.photos[key]
	return o.data, o.contentType, ok
}

// Calls returns every call so far, as "Method key".
func (s *ReceiptPhotos) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

// Fail makes every later call of method ("PresignPut", "Stat", "Head",
// "DeleteUpload" or "Promote") return err; nil heals
// it.
func (s *ReceiptPhotos) Fail(method string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.faults[method] = err
}

// Before runs fn at the start of every later call of method, before its
// fault and its work, with no lock held: a test uses it to change the world
// between two steps, as a concurrent request would.
func (s *ReceiptPhotos) Before(method string, fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.before[method] = fn
}

// enter logs the call, runs its hook and returns its fault. On success it
// returns with s.mu held.
func (s *ReceiptPhotos) enter(method, key string) error {
	s.mu.Lock()
	s.calls = append(s.calls, method+" "+key)
	hook := s.before[method]
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
	s.mu.Lock()
	if err := s.faults[method]; err != nil {
		s.mu.Unlock()
		return err
	}
	return nil
}

// PresignPut implements apppurchasing.ReceiptPhotos.
func (s *ReceiptPhotos) PresignPut(_ context.Context, key, contentType string, ttl time.Duration) (apppurchasing.SignedUpload, error) {
	if err := s.enter("PresignPut", key); err != nil {
		return apppurchasing.SignedUpload{}, err
	}
	defer s.mu.Unlock()
	return apppurchasing.SignedUpload{
		URL: "https://uploads.test/" + key, Method: "PUT",
		Headers: map[string]string{"Content-Type": contentType}, ExpiresAt: s.clock.Now().Add(ttl),
	}, nil
}

// Stat implements apppurchasing.ReceiptPhotos.
func (s *ReceiptPhotos) Stat(_ context.Context, key string) (apppurchasing.ObjectInfo, error) {
	if err := s.enter("Stat", key); err != nil {
		return apppurchasing.ObjectInfo{}, err
	}
	defer s.mu.Unlock()
	o, ok := s.uploads[key]
	if !ok {
		return apppurchasing.ObjectInfo{}, apppurchasing.ErrObjectMissing
	}
	return apppurchasing.ObjectInfo{Size: int64(len(o.data)), ETag: o.etag}, nil
}

// upload returns the staged upload at key at version etag. Call it with
// s.mu held.
func (s *ReceiptPhotos) upload(key, etag string) (object, error) {
	o, ok := s.uploads[key]
	switch {
	case !ok:
		return object{}, apppurchasing.ErrObjectMissing
	case o.etag != etag:
		return object{}, fmt.Errorf("upload %s is at %s, not %s: %w", key, o.etag, etag, apppurchasing.ErrObjectChanged)
	}
	return o, nil
}

// Head implements apppurchasing.ReceiptPhotos.
func (s *ReceiptPhotos) Head(_ context.Context, key, etag string, n int) ([]byte, error) {
	if n < 1 || etag == "" {
		return nil, fmt.Errorf("fake: read %d bytes of %s at etag %q", n, key, etag)
	}
	if err := s.enter("Head", key); err != nil {
		return nil, err
	}
	defer s.mu.Unlock()
	o, err := s.upload(key, etag)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), o.data[:min(n, len(o.data))]...), nil
}

// DeleteUpload implements apppurchasing.ReceiptPhotos.
func (s *ReceiptPhotos) DeleteUpload(_ context.Context, key string) error {
	if err := s.enter("DeleteUpload", key); err != nil {
		return err
	}
	defer s.mu.Unlock()
	delete(s.uploads, key)
	return nil
}

// Promote implements apppurchasing.ReceiptPhotos.
func (s *ReceiptPhotos) Promote(_ context.Context, key, etag, contentType string) error {
	if err := s.enter("Promote", key); err != nil {
		return err
	}
	defer s.mu.Unlock()
	o, err := s.upload(key, etag)
	if err != nil {
		return err
	}
	o.contentType = contentType
	s.photos[key] = o
	return nil
}

// ReportOrphan implements apppurchasing.ReceiptPhotos: it logs the call, which
// Orphans returns.
func (s *ReceiptPhotos) ReportOrphan(_ context.Context, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, "ReportOrphan "+key)
	s.orphans = append(s.orphans, key)
}

// Orphans returns the keys reported as orphans, in order.
func (s *ReceiptPhotos) Orphans() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.orphans...)
}
