package supplier

import (
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

// CreateInput adds a supplier (POST /api/v1/admin/suppliers).
type CreateInput struct {
	Name          string
	WhatsAppPhone string
}

// ReplaceInput replaces a supplier's name and number, with the version read
// (PUT /api/v1/admin/suppliers/{supplierId}).
type ReplaceInput struct {
	ID            shared.SupplierID
	Name          string
	WhatsAppPhone string
	Version       int
}

// ListFilter narrows the list (GET /api/v1/admin/suppliers). The zero value
// lists every supplier.
type ListFilter struct {
	// Query keeps the suppliers whose name contains it, ignoring case.
	Query string
}

// View is a supplier (the Supplier schema). It carries no cost.
type View struct {
	ID            shared.SupplierID
	Name          string
	WhatsAppPhone string
	Version       int
}

func viewOf(s *supplier.Supplier, version int) View {
	return View{ID: s.ID(), Name: s.Name(), WhatsAppPhone: s.WhatsAppPhone(), Version: version}
}
