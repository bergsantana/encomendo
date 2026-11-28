package dto

import (
	"fmt"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// maxPicked is the contract's bound on the items of one request
// (maxItems of SupplierOrderRequest, ReceiptRequest and PaidPricesRequest).
const maxPicked = 200

// SupplierOrderRequest is the body of previewSupplierOrder and
// sendSupplierOrder.
type SupplierOrderRequest struct {
	SupplierID   *string  `json:"supplierId"`
	OrderItemIDs []string `json:"orderItemIds"`
}

// Input returns the picked items, or a ValidationError. An empty pick and an
// item picked twice are the domain's to refuse.
func (r SupplierOrderRequest) Input() (apppurchasing.PickInput, error) {
	var f fields
	in := apppurchasing.PickInput{Supplier: needID[shared.SupplierID](&f, r.SupplierID, "supplierId")}
	ids := needList(&f, r.OrderItemIDs, "orderItemIds")
	if len(ids) > maxPicked {
		f.add("orderItemIds", "Use até 200 itens por vez.")
	}
	in.OrderItems = parseIDs[shared.OrderItemID](&f, ids, "orderItemIds")
	return in, f.err()
}

// SendInput is Input with the Idempotency-Key of the send.
func (r SupplierOrderRequest) SendInput(key appshared.IdempotencyKey) (apppurchasing.SendInput, error) {
	pick, err := r.Input()
	return apppurchasing.SendInput{PickInput: pick, Idempotency: key}, err
}

// PromisedDateRequest is PUT /supplier-orders/{supplierOrderId}/promised-date.
type PromisedDateRequest struct {
	Version *int              `json:"version"`
	Date    *string           `json:"date"`
	Lines   []LineDateRequest `json:"lines"`
}

// LineDateRequest is the promised date of one line.
type LineDateRequest struct {
	LineID *string `json:"lineId"`
	Date   *string `json:"date"`
}

// Input returns the use case's input, or a ValidationError.
func (r PromisedDateRequest) Input(id shared.SupplierOrderID) (apppurchasing.PromisedDateInput, error) {
	var f fields
	in := apppurchasing.PromisedDateInput{ID: id, Version: needVersion(&f, r.Version)}
	in.Date = needDate(&f, r.Date, "date")
	if len(r.Lines) > maxPicked {
		f.add("lines", "Use até 200 itens por vez.")
	}
	for i, l := range r.Lines {
		in.Lines = append(in.Lines, apppurchasing.LineDate{
			Line: needID[shared.PurchaseLineID](&f, l.LineID, fmt.Sprintf("lines[%d].lineId", i)),
			Date: needDate(&f, l.Date, fmt.Sprintf("lines[%d].date", i)),
		})
	}
	return in, f.err()
}

// needDate parses a required CivilDate (YYYY-MM-DD).
func needDate(f *fields, raw *string, field string) shared.CivilDate {
	s := need(f, raw, field)
	if raw == nil {
		return shared.CivilDate{}
	}
	d, err := shared.ParseCivilDate(s)
	if err != nil {
		f.add(field, "Use uma data no formato AAAA-MM-DD.")
	}
	return d
}

// SupplierOrderVersion returns the use case's input of in-transit and undo
// for the supplier order and the version the caller read, or a
// ValidationError.
func SupplierOrderVersion(id shared.SupplierOrderID, r VersionRequest) (apppurchasing.VersionInput, error) {
	v, err := r.Input()
	return apppurchasing.VersionInput{ID: id, Version: v}, err
}

// PaidPricesRequest is PUT /supplier-orders/{supplierOrderId}/paid-prices.
type PaidPricesRequest struct {
	Version *int               `json:"version"`
	Prices  []LinePriceRequest `json:"prices"`
}

// LinePriceRequest is the unit price paid for one line.
type LinePriceRequest struct {
	LineID *string `json:"lineId"`
	Cents  *int64  `json:"cents"`
}

// Input returns the use case's input, or a ValidationError.
func (r PaidPricesRequest) Input(id shared.SupplierOrderID) (apppurchasing.PaidPricesInput, error) {
	var f fields
	in := apppurchasing.PaidPricesInput{ID: id, Version: needVersion(&f, r.Version)}
	prices := needList(&f, r.Prices, "prices")
	if len(prices) == 0 && r.Prices != nil {
		f.add("prices", "Informe ao menos um preço.")
	}
	if len(prices) > maxPicked {
		f.add("prices", "Use até 200 itens por vez.")
	}
	for i, p := range prices {
		in.Prices = append(in.Prices, apppurchasing.LinePrice{
			Line:  needID[shared.PurchaseLineID](&f, p.LineID, fmt.Sprintf("prices[%d].lineId", i)),
			Cents: need(&f, p.Cents, fmt.Sprintf("prices[%d].cents", i)),
		})
	}
	return in, f.err()
}

// ReceiptPhotoUploadRequest is POST /receipt-photo-uploads.
type ReceiptPhotoUploadRequest struct {
	ContentType *string `json:"contentType"`
}

// Input returns the content type, or a ValidationError when it is missing.
// The use case checks the type, as it owns the list.
func (r ReceiptPhotoUploadRequest) Input() (string, error) {
	var f fields
	t := need(&f, r.ContentType, "contentType")
	return t, f.err()
}

// ReceiptEntryRequest is one entry of a receipt: an open line (lineId) or
// an item bought at the counter (orderItemId), never both.
type ReceiptEntryRequest struct {
	LineID      *string  `json:"lineId"`
	OrderItemID *string  `json:"orderItemId"`
	Check       *string  `json:"check"`
	Note        string   `json:"note"`
	PhotoKeys   []string `json:"photoKeys"`
}

// ReceiptRequest is POST /receipts.
type ReceiptRequest struct {
	Entries          []ReceiptEntryRequest `json:"entries"`
	InvoicePhotoKeys []string              `json:"invoicePhotoKeys"`
}

// Input returns the use case's input with the receipt's Idempotency-Key, or
// a ValidationError listing every entry that names neither or both of a
// line and an item.
func (r ReceiptRequest) Input(key appshared.IdempotencyKey) (apppurchasing.ReceiptInput, error) {
	var f fields
	entries := needList(&f, r.Entries, "entries")
	if len(entries) > maxPicked {
		f.add("entries", "Use até 200 itens por recebimento.")
	}
	in := apppurchasing.ReceiptInput{InvoicePhotoKeys: r.InvoicePhotoKeys, Idempotency: key}
	for i, e := range entries {
		in.Entries = append(in.Entries, e.input(&f, i))
	}
	return in, f.err()
}

func (e ReceiptEntryRequest) input(f *fields, i int) apppurchasing.ReceiptEntryInput {
	out := apppurchasing.ReceiptEntryInput{
		Check: purchasing.Check(need(f, e.Check, fmt.Sprintf("entries[%d].check", i))), Note: e.Note, PhotoKeys: e.PhotoKeys,
	}
	// Exactly one target: the contract's oneOf of two entry shapes.
	if (e.LineID == nil) == (e.OrderItemID == nil) {
		f.add(fmt.Sprintf("entries[%d]", i), "Informe a linha ou o item a receber.")
		return out
	}
	if e.LineID != nil {
		out.Line = parseID[shared.PurchaseLineID](f, *e.LineID, fmt.Sprintf("entries[%d].lineId", i))
	} else {
		out.OrderItem = parseID[shared.OrderItemID](f, *e.OrderItemID, fmt.Sprintf("entries[%d].orderItemId", i))
	}
	return out
}
