package customer

import (
	"context"
	"fmt"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/customer"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Service implements UseCases: the store's customers.
type Service struct {
	repo Repository
	tx   appshared.TxManager
	ids  appshared.IDGenerator
}

// NewService returns a Service over its ports. Every one is required.
func NewService(repo Repository, tx appshared.TxManager, ids appshared.IDGenerator) *Service {
	return &Service{repo: repo, tx: tx, ids: ids}
}

// List returns one page of the store's customers, ordered by name and then
// id, narrowed by f (order:read). A cursor the repository did not issue is a
// ValidationError.
func (s *Service) List(ctx context.Context, f ListFilter, p shared.PageRequest) (shared.Page[Summary], error) {
	by, err := authz.Require(ctx, identity.PermOrderRead)
	if err != nil {
		return shared.Page[Summary]{}, err
	}
	var out shared.Page[Summary]
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		page, err := s.repo.List(ctx, by.TenantID(), f, p)
		if err != nil {
			return fmt.Errorf("list customers: %w", err)
		}
		out = page
		return nil
	})
	if err != nil {
		return shared.Page[Summary]{}, err
	}
	return out, nil
}

// Get returns one customer (order:read). It returns customer.ErrNotFound for
// an unknown customer, and for another store's.
func (s *Service) Get(ctx context.Context, id shared.CustomerID) (View, error) {
	by, err := authz.Require(ctx, identity.PermOrderRead)
	if err != nil {
		return View{}, err
	}
	return s.read(ctx, func(ctx context.Context) (*customer.Customer, error) {
		c, err := s.repo.ByID(ctx, by.TenantID(), id)
		if err != nil {
			return nil, fmt.Errorf("load customer %s: %w", id, err)
		}
		return c, nil
	})
}

// ByPhone returns the store's customer with this WhatsApp number
// (order:read), for the order flow. It returns customer.ErrNotFound when the
// store has none.
func (s *Service) ByPhone(ctx context.Context, phone string) (View, error) {
	by, err := authz.Require(ctx, identity.PermOrderRead)
	if err != nil {
		return View{}, err
	}
	return s.read(ctx, func(ctx context.Context) (*customer.Customer, error) {
		c, err := s.repo.ByPhone(ctx, by.TenantID(), phone)
		if err != nil {
			return nil, fmt.Errorf("find customer by phone: %w", err)
		}
		return c, nil
	})
}

// read runs load in a transaction and returns its customer as a view.
func (s *Service) read(ctx context.Context, load func(ctx context.Context) (*customer.Customer, error)) (View, error) {
	var c *customer.Customer
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		loaded, err := load(ctx)
		c = loaded
		return err
	})
	if err != nil {
		return View{}, err
	}
	return viewOf(c, c.Version()), nil
}

// Create adds a customer to the caller's store at version 1 (order:write:
// Novo pedido creates customers). Every address gets a new id. It returns a
// ValidationError listing every invalid field, and ErrPhoneTaken when another
// customer of the store has the number.
func (s *Service) Create(ctx context.Context, in CreateInput) (View, error) {
	by, err := authz.Require(ctx, identity.PermOrderWrite)
	if err != nil {
		return View{}, err
	}
	spec := in.Spec
	spec.Addresses = s.withIDs(spec.Addresses, func(shared.AddressID) bool { return false })
	c, err := customer.New(by.TenantID(), shared.CustomerID(s.ids.NewID()), spec)
	if err != nil {
		return View{}, err
	}
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.Insert(ctx, c); err != nil {
			return fmt.Errorf("insert customer %s: %w", c.ID(), err)
		}
		return nil
	})
	if err != nil {
		return View{}, err
	}
	return viewOf(c, c.Version()), nil
}

// AddAddress adds spec to the customer as a new address with a new id (spec's
// own id is ignored) and stores the next version (order:write: staff may add
// an address from an order, never edit or remove one; ADR 0030, Amendments).
// It returns customer.ErrNotFound for an unknown customer, shared.ErrVersionConflict
// when another write got there first, and a ValidationError naming the bare
// address field ("cep"), or "addresses" past 10 addresses.
func (s *Service) AddAddress(ctx context.Context, id shared.CustomerID, spec customer.AddressSpec) (AddressView, error) {
	by, err := authz.Require(ctx, identity.PermOrderWrite)
	if err != nil {
		return AddressView{}, err
	}
	spec.ID = shared.AddressID(s.ids.NewID())
	var added customer.Address
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		c, err := s.repo.ByID(ctx, by.TenantID(), id)
		if err != nil {
			return fmt.Errorf("load customer %s: %w", id, err)
		}
		if added, err = c.AddAddress(spec); err != nil {
			return err
		}
		if err := s.repo.Update(ctx, c); err != nil { // optimistic lock on version
			return fmt.Errorf("save customer %s: %w", id, err)
		}
		return nil
	})
	if err != nil {
		return AddressView{}, err
	}
	return AddressView(added), nil
}

// Update replaces a customer's fields and addresses (customer:write). An
// address keeps its id only when the customer already has it; any other
// gets a new one. It returns customer.ErrNotFound for an unknown customer,
// shared.ErrVersionConflict when in.Version is not the stored version,
// ErrPhoneTaken when another customer has the number, and a ValidationError
// listing every invalid field. A stale version wins over invalid values: the
// client has to reload first.
func (s *Service) Update(ctx context.Context, in UpdateInput) (View, error) {
	by, err := authz.Require(ctx, identity.PermCustomerWrite)
	if err != nil {
		return View{}, err
	}
	var c *customer.Customer
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		loaded, err := s.repo.ByID(ctx, by.TenantID(), in.ID)
		if err != nil {
			return fmt.Errorf("load customer %s: %w", in.ID, err)
		}
		c = loaded
		spec := in.Spec
		spec.Addresses = s.withIDs(spec.Addresses, func(id shared.AddressID) bool {
			_, owned := c.Address(id)
			return owned
		})
		if err := c.Replace(spec, in.Version); err != nil {
			return err
		}
		if err := s.repo.Update(ctx, c); err != nil { // optimistic lock on version
			return fmt.Errorf("save customer %s: %w", in.ID, err)
		}
		return nil
	})
	if err != nil {
		return View{}, err
	}
	return viewOf(c, c.Version()+1), nil // the repository stored the next version
}

// withIDs returns a copy of specs in which an address keeps its id when keep
// says it may, and gets a new one from the generator otherwise. A repeated
// id that keep allows stays repeated, so the domain reports it.
func (s *Service) withIDs(specs []customer.AddressSpec, keep func(shared.AddressID) bool) []customer.AddressSpec {
	out := make([]customer.AddressSpec, len(specs))
	for i, a := range specs {
		if a.ID.IsZero() || !keep(a.ID) {
			a.ID = shared.AddressID(s.ids.NewID())
		}
		out[i] = a
	}
	return out
}
