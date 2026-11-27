package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres/models"
	appcustomer "github.com/bergsantana/encomendo/backend/internal/app/customer"
	"github.com/bergsantana/encomendo/backend/internal/domain/customer"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// customersPhoneKey is the unique key of (tenant_id, whatsapp_phone) in
// 0012_customers, which Postgres names after its columns.
const customersPhoneKey = "customers_tenant_id_whatsapp_phone_key"

// ordersOfCustomer joins the count and the newest creation time of a
// customer's orders, cancelled ones included. A lateral join reads each
// customer's orders through orders_tenant_customer_idx, and a customer with
// none still gets a row (a count of 0 and no date).
const ordersOfCustomer = `LEFT JOIN LATERAL (
	SELECT count(*) AS orders_count, max(created_at) AS last_order_at
	FROM orders
	WHERE orders.tenant_id = customers.tenant_id AND orders.customer_id = customers.id
) o ON true`

// Customers implements appcustomer.Repository on customers and
// customer_addresses, both under tenant_isolation (0012): a scope sees and
// writes only its own store's rows, with or without a user.
//
// The GORM model leaves client_account_id out of every write, because
// encomendo_app has no grant on it until milestone 7, and every update names
// its columns (the UPDATE grant covers only some).
type Customers struct{}

// NewCustomers returns the customers repository.
func NewCustomers() *Customers { return &Customers{} }

// Insert stores a new customer and its addresses. A number another customer
// of the store has is appcustomer.ErrPhoneTaken; a taken id, or an address
// id in use, is an error matching shared.ErrConflict.
func (*Customers) Insert(ctx context.Context, c *customer.Customer) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	m := models.CustomerFromDomain(c)
	if err := db.Create(&m).Error; err != nil {
		return customerWrite("insert customer", err)
	}
	addresses := models.CustomerAddressesFromDomain(c)
	if len(addresses) == 0 {
		return nil
	}
	return wrap("insert customer addresses", db.Create(&addresses).Error)
}

// Update stores the customer's fields while the stored version is still
// c.Version(), and the next version, then replaces its addresses as a set.
// It returns shared.ErrVersionConflict for a stale version,
// customer.ErrNotFound when the customer is gone or not visible to the
// scope, appcustomer.ErrPhoneTaken for a number another customer has, and an
// error matching shared.ErrInvalid for an address id of another customer.
func (*Customers) Update(ctx context.Context, c *customer.Customer) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	m := models.CustomerFromDomain(c)
	res := db.Model(&models.Customer{}).
		Where("tenant_id = ? AND id = ? AND version = ?", m.TenantID, m.ID, c.Version()).
		Updates(map[string]any{
			"name": m.Name, "whatsapp_phone": m.WhatsAppPhone, "email": m.Email, "notes": m.Notes,
			"version": c.Version() + 1, "updated_at": gorm.Expr("now()"),
		})
	if res.Error != nil {
		return customerWrite("update customer", res.Error)
	}
	if res.RowsAffected == 0 {
		return missingOrStale(db, "customers", c.TenantID(), m.ID, customer.ErrNotFound)
	}
	return replaceAddresses(db, c)
}

// replaceAddresses makes the customer's stored addresses exactly c's: the
// ones c no longer has are deleted and the others upserted with their new
// position and text.
//
// The upsert may update only a row of this customer. An id that belongs to
// another customer matches no row, which would drop the address silently, so
// it is refused as an invalid reference instead: a client's id must never
// reach another customer's address.
func replaceAddresses(db *gorm.DB, c *customer.Customer) error {
	rows := models.CustomerAddressesFromDomain(c)
	del := db.Where("tenant_id = ? AND customer_id = ?", c.TenantID().String(), c.ID().String())
	if len(rows) > 0 {
		ids := make([]string, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
		del = del.Where("id NOT IN ?", ids)
	}
	if err := del.Delete(&models.CustomerAddress{}).Error; err != nil {
		return wrap("delete customer addresses", err)
	}
	if len(rows) == 0 {
		return nil
	}
	res := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		Where:   clause.Where{Exprs: []clause.Expression{gorm.Expr("customer_addresses.customer_id = excluded.customer_id")}},
		DoUpdates: append(
			clause.AssignmentColumns([]string{"position", "label", "cep", "street", "number", "complement", "district", "city", "state"}),
			clause.Assignment{Column: clause.Column{Name: "updated_at"}, Value: gorm.Expr("now()")},
		),
	}).Create(&rows)
	if res.Error != nil {
		return wrap("save customer addresses", res.Error)
	}
	if res.RowsAffected != int64(len(rows)) {
		return fmt.Errorf("save customer addresses: an address id belongs to another customer: %w", shared.ErrInvalid)
	}
	return nil
}

// customerWrite maps the error of a write on customers: the unique number is
// appcustomer.ErrPhoneTaken, anything else as every repository does.
func customerWrite(doing string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == customersPhoneKey {
		return fmt.Errorf("%s: %w: %w", doing, appcustomer.ErrPhoneTaken, err)
	}
	return wrap(doing, err)
}

// ByID returns the customer with its addresses in order, or
// customer.ErrNotFound.
func (*Customers) ByID(ctx context.Context, tenant shared.TenantID, id shared.CustomerID) (*customer.Customer, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	return loadCustomer(db, "tenant_id = ? AND id = ?", tenant.String(), id.String())
}

// ByPhone returns the store's customer with this number, or
// customer.ErrNotFound.
func (*Customers) ByPhone(ctx context.Context, tenant shared.TenantID, phone string) (*customer.Customer, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	return loadCustomer(db, "tenant_id = ? AND whatsapp_phone = ?", tenant.String(), phone)
}

// loadCustomer reads the one customer matching the condition, and its
// addresses by position.
func loadCustomer(db *gorm.DB, cond string, args ...any) (*customer.Customer, error) {
	var m models.Customer
	err := db.Where(cond, args...).Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, customer.ErrNotFound
	}
	if err != nil {
		return nil, wrap("read customer", err)
	}
	var addresses []models.CustomerAddress
	err = db.Where("tenant_id = ? AND customer_id = ?", m.TenantID, m.ID).Order("position").Find(&addresses).Error
	if err != nil {
		return nil, wrap("read customer addresses", err)
	}
	return m.ToDomain(addresses)
}

// List returns one page of the store's customers by lower(name), then id,
// with the count and last date of their orders.
func (*Customers) List(ctx context.Context, tenant shared.TenantID, f appcustomer.ListFilter, page shared.PageRequest) (shared.Page[appcustomer.Summary], error) {
	db, err := conn(ctx)
	if err != nil {
		return shared.Page[appcustomer.Summary]{}, err
	}
	rows, next, err := namePageRows(db, namePage{
		table: "customers", tenant: tenant, req: page,
		narrow: func(q *gorm.DB) *gorm.DB {
			return narrowCustomers(q.Joins(ordersOfCustomer).
				Select("customers.id, customers.name, customers.whatsapp_phone, o.orders_count, o.last_order_at"), f)
		},
	}, func(m models.CustomerSummary) string { return m.ID })
	if err != nil {
		return shared.Page[appcustomer.Summary]{}, err
	}
	out := shared.Page[appcustomer.Summary]{Items: make([]appcustomer.Summary, 0, len(rows)), NextCursor: next}
	for _, m := range rows {
		s, err := m.ToSummary()
		if err != nil {
			return shared.Page[appcustomer.Summary]{}, err
		}
		out.Items = append(out.Items, s)
	}
	return out, nil
}

// narrowCustomers keeps the customers whose name starts with the query,
// ignoring case and taking "%" and "_" literally, or whose number contains
// the query's digits.
func narrowCustomers(q *gorm.DB, f appcustomer.ListFilter) *gorm.DB {
	prefix := f.NamePrefix()
	if prefix == "" {
		return q
	}
	cond, args := "lower(customers.name) LIKE lower(?)", []any{likeEscaper.Replace(prefix) + "%"}
	if digits := f.PhoneDigits(); digits != "" {
		cond += " OR customers.whatsapp_phone LIKE ?"
		args = append(args, "%"+digits+"%")
	}
	return q.Where("("+cond+")", args...)
}
