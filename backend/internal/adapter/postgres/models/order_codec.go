package models

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// The jsonb columns of the order tables are written through the small
// structs below, never through the domain types, so a field added to the
// domain changes what is stored only when someone changes this file, and a
// renamed Go field never silently renames a stored key.

// nilable returns the value p points to, or nil: the way a map handed to
// Updates writes NULL.
func nilable[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

// nilableBytes is nilable for a jsonb column.
func nilableBytes(b []byte) any {
	if b == nil {
		return nil
	}
	return b
}

// optID returns the id as text, or nil for the zero id (a NULL column).
func optID[T ~[16]byte](id T) *string {
	if id == (T{}) {
		return nil
	}
	s := shared.ID(id).String()
	return &s
}

// parseOptID reads a nullable uuid column; NULL is the zero id.
func parseOptID[T ~[16]byte](s *string) (T, error) {
	if s == nil {
		return T{}, nil
	}
	return parseID[T](*s)
}

func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// timeOrNil returns t in UTC, or nil for the zero time.
func timeOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}

// dateValue returns the date as UTC midnight, the value a date column takes.
// An unset date is the zero time, which no column accepts: callers check.
func dateValue(d shared.CivilDate) time.Time {
	if d.IsZero() {
		return time.Time{}
	}
	t, err := time.Parse(time.DateOnly, d.String())
	if err != nil {
		return time.Time{} // unreachable: CivilDate.String is always a valid date
	}
	return t
}

func dateOrNil(d shared.CivilDate) *time.Time {
	if d.IsZero() {
		return nil
	}
	t := dateValue(d)
	return &t
}

func dateOfValue(t time.Time) shared.CivilDate { return shared.DateOf(t, time.UTC) }

func dateOf(t *time.Time) shared.CivilDate {
	if t == nil {
		return shared.CivilDate{}
	}
	return dateOfValue(*t)
}

func centsOrNil(m *shared.Money) *int64 {
	if m == nil {
		return nil
	}
	c := m.Cents()
	return &c
}

func moneyOrNil(cents *int64, currency string) (*shared.Money, error) {
	if cents == nil {
		return nil, nil
	}
	m, err := shared.NewMoney(*cents, currency)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

type addressJSON struct {
	Label      string `json:"label"`
	CEP        string `json:"cep"`
	Street     string `json:"street"`
	Number     string `json:"number"`
	Complement string `json:"complement,omitempty"`
	District   string `json:"district,omitempty"`
	City       string `json:"city"`
	State      string `json:"state"`
}

func marshalAddress(a order.DeliveryAddress) ([]byte, error) {
	return json.Marshal(addressJSON{
		Label: a.Label, CEP: a.CEP, Street: a.Street, Number: a.Number, Complement: a.Complement,
		District: a.District, City: a.City, State: a.State,
	})
}

func unmarshalAddress(id string, raw []byte) (*order.DeliveryAddress, error) {
	var j addressJSON
	if err := json.Unmarshal(raw, &j); err != nil {
		return nil, err
	}
	aid, err := parseID[shared.AddressID](id)
	if err != nil {
		return nil, err
	}
	return &order.DeliveryAddress{
		ID: aid, Label: j.Label, CEP: j.CEP, Street: j.Street, Number: j.Number, Complement: j.Complement,
		District: j.District, City: j.City, State: j.State,
	}, nil
}

type kitJSON struct {
	Name  string     `json:"name"`
	Swaps []swapJSON `json:"swaps,omitempty"`
}

type swapJSON struct {
	Removed     string `json:"removed"`
	Added       string `json:"added"`
	RemovedName string `json:"removedName"`
	AddedName   string `json:"addedName"`
}

func marshalKit(k order.KitSnapshot) ([]byte, error) {
	j := kitJSON{Name: k.Name}
	for _, s := range k.Swaps {
		j.Swaps = append(j.Swaps, swapJSON{
			Removed: s.Removed.String(), Added: s.Added.String(), RemovedName: s.RemovedName, AddedName: s.AddedName,
		})
	}
	return json.Marshal(j)
}

func unmarshalKit(id string, raw []byte) (*order.KitSnapshot, error) {
	var j kitJSON
	if err := json.Unmarshal(raw, &j); err != nil {
		return nil, err
	}
	kid, err := parseID[shared.KitID](id)
	if err != nil {
		return nil, err
	}
	k := &order.KitSnapshot{Kit: kid, Name: j.Name}
	for _, s := range j.Swaps {
		removed, err := parseID[shared.ItemID](s.Removed)
		if err != nil {
			return nil, err
		}
		added, err := parseID[shared.ItemID](s.Added)
		if err != nil {
			return nil, err
		}
		k.Swaps = append(k.Swaps, order.SwapSnapshot{Removed: removed, Added: added, RemovedName: s.RemovedName, AddedName: s.AddedName})
	}
	return k, nil
}

type itemSnapshotJSON struct {
	CatalogItem    string             `json:"catalogItem"`
	Name           string             `json:"name"`
	SourcingMode   string             `json:"sourcingMode"`
	UnitPriceCents int64              `json:"unitPriceCents"`
	Quantity       int                `json:"quantity"`
	Options        []chosenOptionJSON `json:"options,omitempty"`
	LeadDays       int                `json:"leadDays"`
}

type chosenOptionJSON struct {
	Option          string `json:"option"`
	Choice          string `json:"choice"`
	Name            string `json:"name"`
	Label           string `json:"label"`
	ExtraPriceCents int64  `json:"extraPriceCents"`
	ExtraDays       int    `json:"extraDays"`
}

func marshalItemSnapshot(s order.ItemSnapshot) ([]byte, error) {
	j := itemSnapshotJSON{
		CatalogItem: s.CatalogItem.String(), Name: s.Name, SourcingMode: string(s.SourcingMode),
		UnitPriceCents: s.UnitPrice.Cents(), Quantity: s.Quantity, LeadDays: s.LeadDays,
	}
	for _, o := range s.Options {
		j.Options = append(j.Options, chosenOptionJSON{
			Option: o.Option.String(), Choice: o.Choice.String(), Name: o.Name, Label: o.Label,
			ExtraPriceCents: o.ExtraPrice.Cents(), ExtraDays: o.ExtraDays,
		})
	}
	return json.Marshal(j)
}

func unmarshalItemSnapshot(raw []byte, currency string) (order.ItemSnapshot, error) {
	var j itemSnapshotJSON
	if err := json.Unmarshal(raw, &j); err != nil {
		return order.ItemSnapshot{}, err
	}
	item, err := parseID[shared.ItemID](j.CatalogItem)
	if err != nil {
		return order.ItemSnapshot{}, err
	}
	price, err := shared.NewMoney(j.UnitPriceCents, currency)
	if err != nil {
		return order.ItemSnapshot{}, err
	}
	s := order.ItemSnapshot{
		CatalogItem: item, Name: j.Name, SourcingMode: catalog.SourcingMode(j.SourcingMode), UnitPrice: price,
		Quantity: j.Quantity, LeadDays: j.LeadDays,
	}
	for _, o := range j.Options {
		opt, err := parseID[shared.OptionID](o.Option)
		if err != nil {
			return s, err
		}
		choice, err := parseID[shared.ChoiceID](o.Choice)
		if err != nil {
			return s, err
		}
		extra, err := shared.NewMoney(o.ExtraPriceCents, currency)
		if err != nil {
			return s, err
		}
		s.Options = append(s.Options, order.ChosenOption{
			Option: opt, Choice: choice, Name: o.Name, Label: o.Label, ExtraPrice: extra, ExtraDays: o.ExtraDays,
		})
	}
	return s, nil
}

type statusJSON struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	ClientLabel string `json:"clientLabel"`
	Phase       string `json:"phase"`
}

func statusToJSON(s workflow.Status) *statusJSON {
	if s.Key == "" {
		return nil
	}
	return &statusJSON{Key: string(s.Key), Label: s.Label, ClientLabel: s.ClientLabel, Phase: string(s.Phase)}
}

func (j *statusJSON) status() (workflow.Status, error) {
	if j == nil {
		return workflow.Status{}, nil
	}
	phase, err := workflow.ParsePhase(j.Phase)
	if err != nil {
		return workflow.Status{}, err
	}
	return workflow.Status{Key: workflow.StatusKey(j.Key), Label: j.Label, ClientLabel: j.ClientLabel, Phase: phase}, nil
}

// historyDetailJSON is the detail column of order_status_history: the
// HistoryEntry fields its kind uses. It has no supplier name, phone or
// token, as HistoryEntry has none (ADR 0030). The "to" status key is what
// the Hoje query reads to find when an order entered its status.
type historyDetailJSON struct {
	From             *statusJSON `json:"from,omitempty"`
	To               *statusJSON `json:"to,omitempty"`
	Handover         string      `json:"handover,omitempty"`
	Item             string      `json:"item,omitempty"`
	ItemFrom         string      `json:"itemFrom,omitempty"`
	ItemTo           string      `json:"itemTo,omitempty"`
	Check            string      `json:"check,omitempty"`
	Payment          string      `json:"payment,omitempty"`
	PaymentKind      string      `json:"paymentKind,omitempty"`
	PaymentMethod    string      `json:"paymentMethod,omitempty"`
	AmountCents      *int64      `json:"amountCents,omitempty"`
	Currency         string      `json:"currency,omitempty"`
	OldDate          string      `json:"oldDate,omitempty"`
	NewDate          string      `json:"newDate,omitempty"`
	Reason           string      `json:"reason,omitempty"`
	CustomerApproved bool        `json:"customerApproved,omitempty"`
}

func marshalHistoryDetail(e order.HistoryEntry) ([]byte, error) {
	j := historyDetailJSON{
		From: statusToJSON(e.From), To: statusToJSON(e.To), Handover: string(e.Handover),
		ItemFrom: string(e.ItemFrom), ItemTo: string(e.ItemTo), Check: string(e.Check),
		PaymentKind: string(e.PaymentKind), PaymentMethod: string(e.PaymentMethod),
		OldDate: e.OldDate.String(), NewDate: e.NewDate.String(), Reason: e.Reason,
		CustomerApproved: e.CustomerApproved,
	}
	if s := optID(e.Item); s != nil {
		j.Item = *s
	}
	if s := optID(e.Payment); s != nil {
		j.Payment = *s
	}
	if e.Amount.Currency() != "" {
		c := e.Amount.Cents()
		j.AmountCents, j.Currency = &c, e.Amount.Currency()
	}
	return json.Marshal(j)
}

func unmarshalHistoryDetail(raw []byte, e *order.HistoryEntry) error {
	var j historyDetailJSON
	if err := json.Unmarshal(raw, &j); err != nil {
		return err
	}
	var err error
	if e.From, err = j.From.status(); err != nil {
		return err
	}
	if e.To, err = j.To.status(); err != nil {
		return err
	}
	e.Handover, e.ItemFrom, e.ItemTo = order.Handover(j.Handover), order.ItemStatus(j.ItemFrom), order.ItemStatus(j.ItemTo)
	e.Check, e.PaymentKind, e.PaymentMethod = order.Check(j.Check), order.PaymentKind(j.PaymentKind), order.PaymentMethod(j.PaymentMethod)
	e.Reason, e.CustomerApproved = j.Reason, j.CustomerApproved
	if j.Item != "" {
		if e.Item, err = parseID[shared.OrderItemID](j.Item); err != nil {
			return err
		}
	}
	if j.Payment != "" {
		if e.Payment, err = parseID[shared.PaymentID](j.Payment); err != nil {
			return err
		}
	}
	if j.AmountCents != nil {
		if e.Amount, err = shared.NewMoney(*j.AmountCents, j.Currency); err != nil {
			return err
		}
	}
	if j.OldDate != "" {
		if e.OldDate, err = shared.ParseCivilDate(j.OldDate); err != nil {
			return err
		}
	}
	if j.NewDate != "" {
		if e.NewDate, err = shared.ParseCivilDate(j.NewDate); err != nil {
			return err
		}
	}
	return nil
}

// OrderHistory maps order_status_history for inserts. The table's timestamps
// are its defaults and are left out.
type OrderHistory struct {
	TenantID string    `gorm:"column:tenant_id"`
	ID       string    `gorm:"column:id;primaryKey"`
	OrderID  string    `gorm:"column:order_id"`
	Kind     string    `gorm:"column:kind"`
	ByUser   *string   `gorm:"column:by_user"`
	At       time.Time `gorm:"column:at"`
	Detail   []byte    `gorm:"column:detail;type:jsonb"`
}

// TableName returns order_status_history.
func (OrderHistory) TableName() string { return "order_status_history" }

// OrderHistoryFromEntry returns the row of e, with the id the repository
// issued.
func OrderHistoryFromEntry(tenant shared.TenantID, id shared.ID, o shared.OrderID, e order.HistoryEntry) (OrderHistory, error) {
	detail, err := marshalHistoryDetail(e)
	if err != nil {
		return OrderHistory{}, fmt.Errorf("encode history entry %s: %w", e.Kind, err)
	}
	return OrderHistory{
		TenantID: tenant.String(), ID: id.String(), OrderID: o.String(), Kind: string(e.Kind),
		ByUser: optID(e.By), At: e.At.UTC(), Detail: detail,
	}, nil
}

// OrderHistoryRead is a history row with the name of its user, from a left
// join to users. It is only read.
type OrderHistoryRead struct {
	ID     string    `gorm:"column:id"`
	Kind   string    `gorm:"column:kind"`
	ByUser *string   `gorm:"column:by_user"`
	At     time.Time `gorm:"column:at"`
	Detail []byte    `gorm:"column:detail"`
	ByName *string   `gorm:"column:by_name"`
}

// ToEntry rebuilds the entry.
func (m OrderHistoryRead) ToEntry() (order.HistoryEntry, error) {
	e := order.HistoryEntry{Kind: order.HistoryKind(m.Kind), At: m.At.UTC()}
	var err error
	if e.By, err = parseOptID[shared.UserID](m.ByUser); err != nil {
		return e, err
	}
	if err := unmarshalHistoryDetail(m.Detail, &e); err != nil {
		return e, fmt.Errorf("history entry %s: %w", m.ID, err)
	}
	return e, nil
}

// OrderMessage maps order_messages.
type OrderMessage struct {
	TenantID string    `gorm:"column:tenant_id"`
	ID       string    `gorm:"column:id;primaryKey"`
	OrderID  string    `gorm:"column:order_id"`
	Kind     string    `gorm:"column:kind"`
	ToPhone  string    `gorm:"column:to_phone"`
	Text     string    `gorm:"column:text"`
	URL      string    `gorm:"column:url"`
	ByUser   *string   `gorm:"column:by_user"`
	At       time.Time `gorm:"column:at"`
}

// TableName returns order_messages.
func (OrderMessage) TableName() string { return "order_messages" }
