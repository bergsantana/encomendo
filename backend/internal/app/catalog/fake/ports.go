package fake

import (
	"context"
	"sync"
	"time"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Suppliers is an appcatalog.SupplierDirectory over a fixed set of
// suppliers per store. Names answers only for the store in the scope, as
// the real directory does under RLS.
type Suppliers struct {
	mu    sync.Mutex
	names map[shared.TenantID]map[shared.SupplierID]string
	err   error
	calls int
}

var _ appcatalog.SupplierDirectory = (*Suppliers)(nil)

// NewSuppliers returns a directory with no suppliers.
func NewSuppliers() *Suppliers {
	return &Suppliers{names: map[shared.TenantID]map[shared.SupplierID]string{}}
}

// Add gives tenant a supplier.
func (d *Suppliers) Add(tenant shared.TenantID, id shared.SupplierID, name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.names[tenant] == nil {
		d.names[tenant] = map[shared.SupplierID]string{}
	}
	d.names[tenant][id] = name
}

// Fail makes every later Names call return err; nil heals it.
func (d *Suppliers) Fail(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.err = err
}

// Calls returns how many times Names was called.
func (d *Suppliers) Calls() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

// Names implements appcatalog.SupplierDirectory. Like the real one it needs
// a caller with catalog:read.
func (d *Suppliers) Names(ctx context.Context, ids []shared.SupplierID) (map[shared.SupplierID]string, error) {
	if _, err := authz.Require(ctx, identity.PermCatalogRead); err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	if d.err != nil {
		return nil, d.err
	}
	tenant := appshared.ScopeFrom(ctx).TenantID
	out := map[shared.SupplierID]string{}
	for _, id := range ids {
		if name, ok := d.names[tenant][id]; ok {
			out[id] = name
		}
	}
	return out, nil
}

// Defaults is an appcatalog.StoreDefaults with one set of defaults per
// store. Like the real one it needs only a tenant in the scope.
type Defaults struct {
	mu       sync.Mutex
	defaults map[shared.TenantID]appcatalog.Defaults
	err      error
}

var _ appcatalog.StoreDefaults = (*Defaults)(nil)

// NewDefaults returns defaults for no store.
func NewDefaults() *Defaults {
	return &Defaults{defaults: map[shared.TenantID]appcatalog.Defaults{}}
}

// Set gives tenant its defaults.
func (d *Defaults) Set(tenant shared.TenantID, v appcatalog.Defaults) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.defaults[tenant] = v
}

// Fail makes every later call return err; nil heals it.
func (d *Defaults) Fail(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.err = err
}

// StoreDefaults implements appcatalog.StoreDefaults. A store that was never
// Set answers shared.ErrNotFound, which the real port never does: a test
// forgot to set it.
func (d *Defaults) StoreDefaults(ctx context.Context) (appcatalog.Defaults, error) {
	tenant := appshared.ScopeFrom(ctx).TenantID
	if tenant.IsZero() {
		return appcatalog.Defaults{}, authz.ErrTenantRequired
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return appcatalog.Defaults{}, d.err
	}
	v, ok := d.defaults[tenant]
	if !ok {
		return appcatalog.Defaults{}, shared.ErrNotFound
	}
	return v, nil
}

// Photos is an appcatalog.PhotoURLs that signs "https://photos.test/{key}"
// until the clock's now plus the TTL.
type Photos struct {
	mu    sync.Mutex
	clock appshared.Clock
	err   error
}

var _ appcatalog.PhotoURLs = (*Photos)(nil)

// NewPhotos returns a signer on clock.
func NewPhotos(clock appshared.Clock) *Photos { return &Photos{clock: clock} }

// Fail makes every later call return err; nil heals it.
func (p *Photos) Fail(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.err = err
}

// SignGet implements appcatalog.PhotoURLs.
func (p *Photos) SignGet(_ context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return "", time.Time{}, p.err
	}
	return "https://photos.test/" + key, p.clock.Now().Add(ttl), nil
}
