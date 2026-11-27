package fake

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
)

// object is one stored object: its bytes, content type and version.
type object struct {
	data        []byte
	contentType string
	etag        string
}

// Storage is an in-memory appcatalog.PhotoStore with the two places of the
// real one: staged uploads, and promoted photos. Signed URLs are
// "https://uploads.test/{key}" and "https://photos.test/{key}", valid until
// the clock's now plus the TTL. It logs every call, so a test can check
// that a refused key never reached storage.
type Storage struct {
	mu      sync.Mutex
	clock   appshared.Clock
	uploads map[string]object
	photos  map[string]object
	calls   []string
	faults  map[string]error
	before  map[string]func()
	nextTag int
}

var _ appcatalog.PhotoStore = (*Storage)(nil)

// NewStorage returns an empty store on clock.
func NewStorage(clock appshared.Clock) *Storage {
	return &Storage{
		clock: clock, uploads: map[string]object{}, photos: map[string]object{},
		faults: map[string]error{}, before: map[string]func(){},
	}
}

// Upload stages data at key, as an app's signed PUT would, with a new
// version each time.
func (s *Storage) Upload(key, contentType string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextTag++
	s.uploads[key] = object{data: append([]byte(nil), data...), contentType: contentType, etag: "v" + strconv.Itoa(s.nextTag)}
}

// Uploaded reports whether an upload is staged at key.
func (s *Storage) Uploaded(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.uploads[key]
	return ok
}

// Promoted returns the photo object at key and its content type.
func (s *Storage) Promoted(key string) ([]byte, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.photos[key]
	return o.data, o.contentType, ok
}

// Calls returns every call so far, as "Method key".
func (s *Storage) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

// Fail makes every later call of method ("PresignPut", "Stat", "Head",
// "DeleteUpload", "Promote", "Delete" or "SignGet") return err; nil heals
// it.
func (s *Storage) Fail(method string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.faults[method] = err
}

// Before runs fn at the start of every later call of method, before its
// fault and its work, with no lock held: a test uses it to change the world
// between two steps, as a concurrent request would.
func (s *Storage) Before(method string, fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.before[method] = fn
}

// enter logs the call, runs its hook and returns its fault. On success it
// returns with s.mu held.
func (s *Storage) enter(method, key string) error {
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

// SignGet implements appcatalog.PhotoURLs.
func (s *Storage) SignGet(_ context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	if err := s.enter("SignGet", key); err != nil {
		return "", time.Time{}, err
	}
	defer s.mu.Unlock()
	return "https://photos.test/" + key, s.clock.Now().Add(ttl), nil
}

// PresignPut implements appcatalog.PhotoStore.
func (s *Storage) PresignPut(_ context.Context, key, contentType string, ttl time.Duration) (appcatalog.SignedUpload, error) {
	if err := s.enter("PresignPut", key); err != nil {
		return appcatalog.SignedUpload{}, err
	}
	defer s.mu.Unlock()
	return appcatalog.SignedUpload{
		URL: "https://uploads.test/" + key, Method: "PUT",
		Headers: map[string]string{"Content-Type": contentType}, ExpiresAt: s.clock.Now().Add(ttl),
	}, nil
}

// Stat implements appcatalog.PhotoStore.
func (s *Storage) Stat(_ context.Context, key string) (appcatalog.ObjectInfo, error) {
	if err := s.enter("Stat", key); err != nil {
		return appcatalog.ObjectInfo{}, err
	}
	defer s.mu.Unlock()
	o, ok := s.uploads[key]
	if !ok {
		return appcatalog.ObjectInfo{}, appcatalog.ErrObjectMissing
	}
	return appcatalog.ObjectInfo{Size: int64(len(o.data)), ETag: o.etag}, nil
}

// upload returns the staged upload at key at version etag. Call it with
// s.mu held.
func (s *Storage) upload(key, etag string) (object, error) {
	o, ok := s.uploads[key]
	switch {
	case !ok:
		return object{}, appcatalog.ErrObjectMissing
	case o.etag != etag:
		return object{}, fmt.Errorf("upload %s is at %s, not %s: %w", key, o.etag, etag, appcatalog.ErrObjectChanged)
	}
	return o, nil
}

// Head implements appcatalog.PhotoStore.
func (s *Storage) Head(_ context.Context, key, etag string, n int) ([]byte, error) {
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

// DeleteUpload implements appcatalog.PhotoStore.
func (s *Storage) DeleteUpload(_ context.Context, key string) error {
	if err := s.enter("DeleteUpload", key); err != nil {
		return err
	}
	defer s.mu.Unlock()
	delete(s.uploads, key)
	return nil
}

// Promote implements appcatalog.PhotoStore.
func (s *Storage) Promote(_ context.Context, key, etag, contentType string) error {
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

// Delete implements appcatalog.PhotoStore.
func (s *Storage) Delete(_ context.Context, key string) error {
	if err := s.enter("Delete", key); err != nil {
		return err
	}
	defer s.mu.Unlock()
	delete(s.photos, key)
	return nil
}
