# Milestone 4 — code design

Written 2026-11-27 on Opus, with `docs/plans/m4-customers-orders-deadlines.md` (the steps) and ADR 0030 (the decisions). This file fixes the code's shape in advance, so the milestone runs in one session without stopping to design: packages, types, signatures, SQL, routes, problem codes, configuration, web files and test names.

Rules for using it:

- It is binding on names, signatures, tables, routes and codes. Bodies, private helpers and test cases beyond those listed are the builder's.
- A builder that finds a signature wrong (it doesn't compile against `main`, or a rule can't hold with it) fixes the smallest thing that works, notes it in the PR description under "Design changes", and amends this file in the same PR. It does not stop to ask unless the change touches RLS, tokens or what a client can see.
- Everything here follows `backend/CLAUDE.md` and `web/CLAUDE.md`. Doc comments, `doc.go`, error wrapping and test naming are not repeated per item.
- Signatures below omit doc comments to stay short. Every exported name gets one when built.

---

## 1. Domain

### 1.1 `domain/shared` additions

`id.go` — new typed IDs, same pattern as `WorkflowID` (`ParseX`, `String`, `IsZero`, `MarshalText`, `UnmarshalText`):

```go
type CustomerID ID
type AddressID ID
type OrderID ID
type OrderLineID ID
type OrderItemID ID
type PaymentID ID
```

`date.go` — civil dates, used by `schedule`, `order` and the views:

```go
// CivilDate is a calendar day with no time and no zone. The zero value is unset.
type CivilDate struct{ y int; m time.Month; d int }

func NewCivilDate(y int, m time.Month, d int) (CivilDate, error) // ErrInvalid for 2026-02-30 and years outside 2000..2200
func ParseCivilDate(s string) (CivilDate, error)                 // "YYYY-MM-DD" only
func DateOf(t time.Time, loc *time.Location) CivilDate           // the day t falls on in loc
func (d CivilDate) AddDays(n int) CivilDate
func (d CivilDate) DaysUntil(o CivilDate) int // o − d, negative when o is earlier
func (d CivilDate) Before(o CivilDate) bool
func (d CivilDate) After(o CivilDate) bool
func (d CivilDate) IsZero() bool
func (d CivilDate) String() string // "YYYY-MM-DD"; "" when zero
func (d CivilDate) MarshalText() ([]byte, error)
func (d *CivilDate) UnmarshalText(b []byte) error
func Latest(ds ...CivilDate) CivilDate // zero when ds is empty
```

Tests: `TestCivilDate_AddDays_CrossesMonthAndYear`, `TestDateOf_ManausAndSaoPauloDifferAt0330UTC`, `FuzzParseCivilDate` (round trip).

### 1.2 `domain/identity/permission.go`

```go
PermOrderManage   Permission = "order:manage"   // owner, admin
PermOrderOverride Permission = "order:override" // owner
PermCustomerWrite Permission = "customer:write" // owner, admin
```

Add them to the owner and admin lists in `grantsOf`, sorted. Staff and viewer gain nothing. `TestGrants` gets a row per new permission and role.

### 1.3 `domain/tenancy`

`timezone.go`:

```go
type TimeZone string

const (
	ZoneNoronha   TimeZone = "America/Noronha"
	ZoneSaoPaulo  TimeZone = "America/Sao_Paulo" // the default
	ZoneManaus    TimeZone = "America/Manaus"
	ZoneRioBranco TimeZone = "America/Rio_Branco"
)

func Zones() []TimeZone                             // in that order
func (z TimeZone) Valid() bool
func (z TimeZone) Location() (*time.Location, error) // time.LoadLocation; cmd/api embeds time/tzdata
```

`settings.go`: `Editable` gains `TimeZone TimeZone`; `Validate` refuses an invalid one (`Field: "timeZone"`, "Escolha o fuso horário."); `DefaultSettings` sets `ZoneSaoPaulo`. `Settings` gains:

```go
func (s *Settings) ChooseWorkflow(w WorkflowRef, expectedVersion int) error // ErrInvalidSettings for a zero ref; ErrVersionConflict
```

`events.go`: `WorkflowChosen{Tenant shared.TenantID; Workflow shared.WorkflowID; Version int}`, name `tenancy.workflow_chosen`.

### 1.4 `domain/workflow`

Files: `doc.go`, `phase.go`, `status.go`, `workflow.go`, `validate.go`, `moves.go`, `errors.go`, tests, and `presets_test.go` with the four presets as `0003_workflows` seeds them (copied by hand; the integration test in 4.8 checks they match the database).

```go
type Phase string

const (
	PhaseDraft           Phase = "draft"
	PhaseAwaitingDeposit Phase = "awaiting_deposit"
	PhaseConfirmed       Phase = "confirmed"
	PhaseSourcing        Phase = "sourcing"
	PhaseAssembling      Phase = "assembling"
	PhaseReady           Phase = "ready"
	PhaseHandedOver      Phase = "handed_over"
	PhaseCancelled       Phase = "cancelled"
)

func ParsePhase(s string) (Phase, error)
func (p Phase) Rank() int // draft 1 … handed_over 7; cancelled 0

type StatusKey string

type Status struct {
	Key         StatusKey
	Label       string // staff, pt-BR
	ClientLabel string // store page, pt-BR
	Phase       Phase
}

// Cancelled is the implicit status every workflow ends in when an order is
// cancelled. It is never a row; its key is reserved.
var Cancelled = Status{Key: "cancelled", Label: "Cancelado", ClientLabel: "Pedido cancelado", Phase: PhaseCancelled}

type Workflow struct { /* id, version, key, name, statuses (ordered) */ }

func New(id shared.WorkflowID, version int, key, name string, statuses []Status) (Workflow, error) // runs Validate
func (w Workflow) ID() shared.WorkflowID
func (w Workflow) Version() int
func (w Workflow) Key() string
func (w Workflow) Name() string
func (w Workflow) Statuses() []Status
func (w Workflow) Status(k StatusKey) (Status, bool) // Cancelled for "cancelled"
func (w Workflow) First() Status
func (w Workflow) HasPhase(p Phase) bool
```

`validate.go`:

```go
func Validate(statuses []Status) error
```

Returns `ErrInvalidWorkflow` wrapped with the reason: empty; duplicate key; key `cancelled` or not `^[a-z][a-z0-9_]{1,62}$`; label or client label outside 1–60 runes; phase `cancelled` or unknown; a phase lower than the one before it; any of `draft`, `awaiting_deposit`, `ready`, `handed_over` missing; first status not `draft`; last not `handed_over`.

`moves.go`:

```go
func (w Workflow) CanMove(from, to StatusKey) error          // ErrUnknownStatus, ErrInvalidTransition
func (w Workflow) PhasesEntered(from, to StatusKey) []Phase // phases of statuses after from up to to, deduplicated, in order; nil for a move back or to Cancelled
func (w Workflow) Moves(from StatusKey) []Status            // every status CanMove allows, Cancelled last unless from is handed_over
```

Rules: forward to any later status; back only to an earlier status of the same phase; `to == from` is `ErrInvalidTransition`; to `cancelled` from any phase before `handed_over`; nothing from `handed_over` or `cancelled`.

`errors.go`: `ErrInvalidWorkflow` (matches `shared.ErrInvalid`), `ErrUnknownStatus` (matches `shared.ErrInvalid`), `ErrInvalidTransition`.

Tests: `TestValidate_<Rule>` one per rule; `TestPresets_Valid`; `TestPresets_Moves` — for each preset, a table over every `(from, to)` pair including `cancelled`, built from the expected rule, not from `CanMove` itself; `FuzzValidate_ShuffledPresets` (a shuffle that puts a phase back must fail).

### 1.5 `domain/schedule`

Files added: `readiness.go`, `deadline.go`, `alerts.go` (+ tests). `days.go` stays.

```go
// Timing is what an item's readiness depends on.
type Timing struct {
	LeadDays  int               // supplier lead days (to_order, and stock until M6) or production days (in_house)
	ExtraDays int               // the chosen options' extra days
	Promised  shared.CivilDate  // the supplier's promised date, zero until M5 records one
}

func ExpectedDate(t Timing, release shared.CivilDate) shared.CivilDate
// promised if set, else release + LeadDays; then + ExtraDays.

func Delivery(expected []shared.CivilDate, release shared.CivilDate, assembly, margin int) shared.CivilDate
// Latest(expected..., release) + assembly + margin. Panics never; negative days are clamped at 0 by callers' validation.

func ArrivalLimit(promise shared.CivilDate, assembly int) shared.CivilDate // promise − assembly

type Alert string

const (
	AlertAtRisk Alert = "at_risk" // Pode atrasar
	AlertLate   Alert = "late"    // Atrasada
)

type ItemState struct {
	Expected shared.CivilDate
	Arrived  bool // received, ready or installed
}

func Alerts(s ItemState, limit, today shared.CivilDate) []Alert
// nil when arrived; at_risk when Expected after limit; late when Expected before today; both can apply.
```

The workflow-without-assembling rule (0 assembly days) is the caller's: `order` passes 0 when `!wf.HasPhase(PhaseAssembling)`.

Tests: `TestDelivery_KitScramblerCB500X` (release 2026-12-01, seat 15 days, others 5 and 7, assembly 3, margin 2 → 2026-12-21; limit 2026-12-18 — i.e. day 20 and day 17); `TestExpectedDate_PromisedReplacesLead`; `TestExpectedDate_AddsOptionDays`; `TestAlerts_` table (on time, at risk, late, both, arrived); `FuzzDelivery_NeverBeforeRelease`.

### 1.6 `domain/customer`

Files: `doc.go`, `customer.go`, `address.go`, `errors.go`, tests.

```go
type AddressSpec struct {
	ID                                                 shared.AddressID // set by the app layer, new or kept
	Label, CEP, Street, Number, Complement, District, City, State string
}

type Spec struct {
	Name, Phone, Email, Notes string
	Addresses                 []AddressSpec
}

type Customer struct { /* tenant, id, name, phone, email, notes, addresses, clientAccount, version */ }

func New(tenant shared.TenantID, id shared.CustomerID, s Spec) (*Customer, error)
func (c *Customer) Replace(s Spec, expectedVersion int) error // ValidationError; ErrVersionConflict
func (c *Customer) Address(id shared.AddressID) (Address, bool)
// getters: TenantID, ID, Name, Phone, Email, Notes, Addresses, ClientAccountID, Version

type Record struct { /* every field, for repositories */ }
func Restore(r Record) (*Customer, error)
func FirstName(name string) string // for WhatsApp greetings: up to the first space
```

Validation fields and messages (from the design document, Clientes): `name` 1–120, no control characters; `phone` via `shared.ValidWhatsAppPhone`; `email` via `identity.ParseEmail` when not empty; `notes` ≤ 2000; ≤ 10 addresses; `cep` 8 digits; `state` one of the 27 UF codes; `label` 1–40; `street`, `city` required in an address.

### 1.7 `domain/order`

Files: `doc.go`, `order.go`, `number.go`, `line.go`, `item.go`, `item_status.go`, `snapshot.go`, `terms.go`, `pricing.go`, `payment.go`, `transitions.go`, `change.go`, `dates.go`, `cancel.go`, `tracking_token.go`, `history.go`, `messages.go`, `events.go`, `record.go`, `errors.go`, tests. 4.5 builds through `pricing.go` and `item_status.go`; 4.6 builds the rest.

**Values**

```go
type Number int
func (n Number) String() string // "P-0001"; five digits from 10000

type Handover string
const (
	HandoverPickup   Handover = "pickup"
	HandoverDelivery Handover = "delivery"
)

type ItemStatus string
const (
	ItemToBuy          ItemStatus = "to_buy"
	ItemSentToSupplier ItemStatus = "sent_to_supplier"
	ItemInTransit      ItemStatus = "in_transit"
	ItemReceived       ItemStatus = "received"
	ItemPlanned        ItemStatus = "planned"
	ItemInProduction   ItemStatus = "in_production"
	ItemReady          ItemStatus = "ready"
	ItemInstalled      ItemStatus = "installed"
)

type Check string
const (
	CheckOK        Check = "ok"
	CheckDefective Check = "defective"
)

type PaymentKind string   // "deposit", "balance"
type PaymentMethod string // "pix", "dinheiro", "cartao_credito", "cartao_debito", "transferencia", "outro"
```

**Snapshots** (`snapshot.go`). The client-visible snapshot and the staff-only one are separate types so no code path can put a cost in a client view by accident:

```go
type ChosenOption struct {
	Option     shared.OptionID
	Choice     shared.ChoiceID
	Name       string // option name
	Label      string // choice label
	ExtraPrice shared.Money
	ExtraDays  int
}

// ItemSnapshot is what the client may see of an item at order time.
type ItemSnapshot struct {
	CatalogItem  shared.ItemID
	Name         string
	SourcingMode catalog.SourcingMode
	UnitPrice    shared.Money // single-item price, options excluded
	Quantity     int          // 1..99
	Options      []ChosenOption
	LeadDays     int          // ready days at order time (main supplier or production days)
}

// Sourcing is the staff-only part, stored in order_item_sourcing.
type Sourcing struct {
	Supplier     shared.SupplierID // zero for in_house
	SupplierName string
	Cost         *shared.Money     // per unit, nil when unknown
	Backup       *BackupSource
}

type BackupSource struct {
	Supplier shared.SupplierID
	Name     string
	LeadDays int
	Cost     *shared.Money
}

type KitSnapshot struct {
	Kit   shared.KitID
	Name  string
	Swaps []SwapSnapshot
}

type SwapSnapshot struct {
	Removed, Added         shared.ItemID
	RemovedName, AddedName string
}
```

**Lines and items** (`line.go`, `item.go`):

```go
type LineSpec struct {
	ID    shared.OrderLineID
	Kit   *KitSnapshot // nil for a loose item
	Price shared.Money // the line's price, options included; the catalog computes it
	Items []ItemSpec   // a loose-item line has exactly one
	AssemblyDays, SafetyMarginDays *int // a kit's own overrides
}

type ItemSpec struct {
	ID       shared.OrderItemID
	Snapshot ItemSnapshot
	Sourcing Sourcing
}

type Line struct { /* id, kit, price, items */ }
type Item struct { /* id, snapshot, sourcing, status, check, expected, arrivedOn */ }

func (i Item) ID() shared.OrderItemID
func (i Item) Snapshot() ItemSnapshot
func (i Item) Sourcing() Sourcing
func (i Item) Status() ItemStatus
func (i Item) Check() (Check, bool)
func (i Item) Expected() shared.CivilDate
func (i Item) Arrived() bool // received with ok, ready, installed
```

**Item lifecycle** (`item_status.go`):

```go
func InitialStatus(m catalog.SourcingMode) ItemStatus // to_buy for to_order and stock (ADR 0030), planned for in_house
func CanMoveItem(m catalog.SourcingMode, from, to ItemStatus, check *Check) error // ErrInvalidItemMove
```

- `to_order`/`stock`: `to_buy → sent_to_supplier → in_transit → received → installed`; `received` needs a check; `received` with `defective` lands on `to_buy` (the item's status becomes `to_buy`, the check is kept in history); `to_buy → received` is allowed (bought at the counter), as is `sent_to_supplier → received`.
- `in_house`: `planned → in_production → ready → installed`; `ready` with `defective` goes back to `in_production`.
- One step back is allowed for a mistake (`sent_to_supplier → to_buy`, `in_transit → sent_to_supplier`, `in_production → planned`), never from `installed`.

**Terms** (`terms.go`):

```go
type Terms struct {
	Handover         Handover
	Address          shared.AddressID // required for delivery
	Freight          shared.Money     // zero for pickup
	Discount         shared.Money
	DepositPercent   int // 0..100
	AssemblyDays     int // 0..60, the effective number (order > kit > store)
	SafetyMarginDays int // 0..60
}
```

**The aggregate** (`order.go`, `pricing.go`):

```go
type NewSpec struct {
	Tenant   shared.TenantID
	ID       shared.OrderID
	Number   Number
	Customer shared.CustomerID
	Workflow workflow.Workflow
	Lines    []LineSpec
	Terms    Terms
	Tracking shared.SecretHash
	By       shared.Principal
	Now      time.Time
	Today    shared.CivilDate // in the store's zone
}

func New(s NewSpec) (*Order, error)
// Starts at Workflow.First() (draft). Release date unset. Every item's expected date and the promise are forecast from Today.

// getters
func (o *Order) ID() shared.OrderID
func (o *Order) TenantID() shared.TenantID
func (o *Order) Number() Number
func (o *Order) Customer() shared.CustomerID
func (o *Order) Workflow() workflow.Workflow
func (o *Order) Status() workflow.Status
func (o *Order) Phase() workflow.Phase
func (o *Order) Lines() []Line
func (o *Order) Items() []Item
func (o *Order) Terms() Terms
func (o *Order) Payments() []Payment
func (o *Order) ReleaseDate() (shared.CivilDate, bool)
func (o *Order) Released() (by shared.UserID, ok bool) // released without the deposit
func (o *Order) Promise() shared.CivilDate             // delivery_date
func (o *Order) PromiseChangedByHand() bool // API: deliveryDateChanged
func (o *Order) ArrivalLimit() shared.CivilDate
func (o *Order) Alerts(today shared.CivilDate) map[shared.OrderItemID][]schedule.Alert
func (o *Order) CancelReason() (string, bool)
func (o *Order) TrackingHash() shared.SecretHash
func (o *Order) Version() int
func (o *Order) CheckVersion(expected int) error // ErrVersionConflict

// money
func (o *Order) Subtotal() shared.Money    // Σ line prices
func (o *Order) Total() shared.Money       // subtotal + freight − discount
func (o *Order) DepositDue() shared.Money  // total × percent, half up to the cent
func (o *Order) Paid() shared.Money        // Σ payments not voided
func (o *Order) DepositPaid() bool         // Σ deposit payments ≥ DepositDue, or DepositDue is zero
func (o *Order) BalanceDue() shared.Money  // max(total − paid, 0)
func (o *Order) RefundDue() shared.Money   // max(paid − total, 0): "Valor a devolver"
func (o *Order) Cost() (shared.Money, bool)   // Σ unit cost × quantity; false when any is unknown
func (o *Order) Margin() (shared.Money, int, bool) // cents and basis points, toward zero; false without Cost
```

Validation in `New` and `Change`: at least one line; a loose-item line has one item; quantity 1..99; one currency everywhere; discount ≥ 0 and ≤ subtotal (`Field: "discount"`, "O desconto não pode passar do valor dos itens."); freight ≥ 0 and zero for pickup; delivery needs an address ("Escolha o endereço de entrega."); deposit percent 0..100; days 0..60.

**Dates** (`dates.go`):

```go
func (o *Order) assemblyForDeadline() int // Terms.AssemblyDays, or 0 when the workflow has no assembling
func (o *Order) redate(today shared.CivilDate) // recompute every item's expected date from the release date (or today before release)
func (o *Order) forecast() shared.CivilDate    // schedule.Delivery over the items
func (o *Order) ChangeDeliveryDate(date shared.CivilDate, reason string, by shared.Principal, today shared.CivilDate, now time.Time) (old shared.CivilDate, err error)
// ErrDateInPast before today; ErrOrderClosed after handover or cancel. Sets PromiseChangedByHand.
```

The promise is `forecast()` at `New`, at every `Change` before the release, and at the deposit or release, unless `PromiseChangedByHand`. After the release, a `Change` whose forecast is later than the promise moves the promise to it (ADR 0030); `MoveItem` and `UseBackupSupplier` never move it.

**Payments** (`payment.go`):

```go
type PaymentSpec struct {
	ID     shared.PaymentID
	Kind   PaymentKind
	Amount shared.Money
	Method PaymentMethod
	By     shared.Principal
	At     time.Time
	Today  shared.CivilDate
}

type Payment struct { /* id, kind, amount, method, by, at, voided */ }

func (o *Order) RecordPayment(s PaymentSpec) (depositConfirmed bool, err error)
// ErrOrderClosed; amount > 0 (ValidationError); amount > BalanceDue() → ErrPaymentExceedsDue.
// When it makes DepositPaid() true for the first time: release date = Today, redate, promise (unless set by owner).

func (o *Order) VoidPayment(id shared.PaymentID, reason string, by shared.Principal, now time.Time) error
// ErrPaymentNotFound; already void → ErrInvalid; reason 1..500. Voiding a deposit never undoes the release.
```

**Transitions** (`transitions.go`):

```go
func (o *Order) Release(by shared.Principal, today shared.CivilDate, now time.Time) error
// Only in awaiting_deposit or draft, only when !DepositPaid(); sets release date, redates, promise. The permission is the app's.

func (o *Order) Advance(to workflow.StatusKey, handover *Handover, by shared.Principal, now time.Time) error
// CanMove, then the guard of every phase in PhasesEntered:
//   leaving awaiting_deposit (entering confirmed, sourcing or later): DepositPaid() || released → else GuardError{GuardDepositUnpaid}
//   entering assembling, or ready when the workflow has no assembling: every item Arrived() → else GuardError{GuardItemsNotReady}
//   entering handed_over: BalanceDue() zero and handover given → else GuardError{GuardBalanceUnpaid}; ErrHandoverRequired
// Moving to Cancelled goes through Cancel, never Advance (ErrInvalidTransition).

func (o *Order) MoveItem(id shared.OrderItemID, to ItemStatus, check *Check, today shared.CivilDate, now time.Time, by shared.Principal) error
// ErrItemNotFound, ErrInvalidItemMove, ErrOrderClosed. Records arrivedOn on received/ready.

func (o *Order) UseBackupSupplier(id shared.OrderItemID, today shared.CivilDate, now time.Time, by shared.Principal) error
// Only to_order/stock items in to_buy with a Backup: ErrNoBackupSupplier. Swaps Sourcing (old main becomes nothing), LeadDays, redates that item.

type Guard string
const (
	GuardDepositUnpaid Guard = "deposit_unpaid"
	GuardItemsNotReady Guard = "items_not_ready"
	GuardBalanceUnpaid Guard = "balance_unpaid"
)

type GuardError struct{ Guard Guard }
func (e *GuardError) Error() string
func (e *GuardError) Unwrap() error // ErrGuardFailed
```

**Change** (`change.go`):

```go
type ChangeSpec struct {
	Lines            []LineSpec
	Terms            Terms
	CustomerApproved bool
	By               shared.Principal
	Now              time.Time
	Today            shared.CivilDate
}

type ChangeResult struct {
	Paid                    bool // the order had a payment or was past awaiting_deposit
	OldTotal, NewTotal      shared.Money
	OldPromise, NewPromise  shared.CivilDate
	RefundDue               shared.Money
}

func (o *Order) Change(s ChangeSpec) (ChangeResult, error)
```

- Closed (handed over, cancelled) → `ErrOrderClosed`. In `assembling` or later (or `ready` without assembling) → `ErrNotEditable`.
- "Paid" = any non-void payment, or the phase is past `awaiting_deposit`, or released. Paid and `!CustomerApproved` → `ErrApprovalRequired`.
- Items that survive a change (same `OrderItemID`) keep status, check and arrival; new items start at `InitialStatus`. Removing an item that already arrived is allowed; M5 deals with its purchase line.
- Reprice, redate, then the promise rule above.

**Cancel, tracking, history** (`cancel.go`, `tracking_token.go`, `history.go`):

```go
func (o *Order) Cancel(reason string, by shared.Principal, now time.Time) error // reason 1..500; ErrOrderClosed
func (o *Order) RotateTracking(h shared.SecretHash, by shared.Principal, now time.Time)

type HistoryKind string // "created", "status", "item", "payment", "payment_voided", "released", "date", "changed", "tracking_link", "cancelled"

type HistoryEntry struct {
	Kind       HistoryKind
	By         shared.UserID // zero for the store page (M7)
	At         time.Time
	From, To   workflow.Status   // status moves
	Item       shared.OrderItemID
	ItemFrom, ItemTo ItemStatus
	Check      Check
	Payment    shared.PaymentID
	Amount     shared.Money
	OldDate, NewDate shared.CivilDate
	Reason     string
	CustomerApproved bool
}

func (o *Order) PendingHistory() []HistoryEntry // appended since New or Restore; the repository writes them
```

**Messages** (`messages.go`): the pt-BR WhatsApp texts of the design document ("WhatsApp messages"), as pure functions, so tests can assert they never carry a supplier or "Pode atrasar":

```go
type MessageFacts struct {
	Store, CustomerFirstName, Number string
	Total, Deposit, Refund      shared.Money
	OldDate, NewDate            shared.CivilDate
	Reason, TrackingURL         string
}

func TrackingMessage(f MessageFacts) string
func CreatedMessage(f MessageFacts) string
func DateChangedMessage(f MessageFacts) string
func ChangedMessage(f MessageFacts) string
func formatMoney(m shared.Money) string // "R$ 1.234,56"
func formatDate(d shared.CivilDate) string // "dd/mm/aaaa"
```

**Events** (`events.go`), ids and phases only, never a name, phone or amount of a person:

```go
type Created          struct{ Order shared.OrderID; Number Number; Workflow shared.WorkflowID; Version int }       // order.created
type Advanced         struct{ Order shared.OrderID; From, To workflow.StatusKey; Phase workflow.Phase }        // order.advanced
type DepositConfirmed struct{ Order shared.OrderID; ReleaseDate shared.CivilDate }                               // order.deposit_confirmed
type Released         struct{ Order shared.OrderID; ReleaseDate shared.CivilDate }                               // order.released
type PaymentRecorded  struct{ Order shared.OrderID; Payment shared.PaymentID; Kind PaymentKind }                 // order.payment_recorded
type PaymentVoided    struct{ Order shared.OrderID; Payment shared.PaymentID }                                   // order.payment_voided
type ItemMoved        struct{ Order shared.OrderID; Item shared.OrderItemID; To ItemStatus }                     // order.item_moved
type Changed          struct{ Order shared.OrderID; Added, Removed []shared.OrderItemID }                        // order.changed
type DateChanged      struct{ Order shared.OrderID; From, To shared.CivilDate }                                 // order.date_changed
type Cancelled        struct{ Order shared.OrderID }                                                             // order.cancelled
```

**Record** (`record.go`): `Record` (every field, `LineRecord`, `ItemRecord`, `PaymentRecord`) and `Restore(r Record, wf workflow.Workflow) (*Order, error)`; `Restore` refuses a record whose workflow id and version don't match `wf`.

**Errors** (`errors.go`):

```go
ErrInvalidTransition  // = workflow.ErrInvalidTransition re-exported
ErrGuardFailed
ErrNotEditable
ErrApprovalRequired   // matches shared.ErrInvalid
ErrPaymentExceedsDue  // matches shared.ErrInvalid
ErrPaymentNotFound    // matches shared.ErrNotFound
ErrItemNotFound       // matches shared.ErrNotFound
ErrInvalidItemMove
ErrNoBackupSupplier
ErrHandoverRequired   // matches shared.ErrInvalid
ErrDateInPast         // matches shared.ErrInvalid
ErrOrderClosed
```

**Tests** (named; table tests unless stated):

- `TestNew_StartsAtFirstDraftStatus_ForEveryPreset`
- `TestNew_RefusesEmptyLinesMixedCurrencyDiscountAboveSubtotal`
- `TestTotals_KitKeepAndLoseWithOptionsFreightDiscount`
- `TestDepositDue_RoundsHalfUp`
- `TestCanMoveItem_EveryPairPerSourcingMode`
- `TestMoveItem_DefectiveGoesBack`
- `TestAdvance_<Preset>_EveryMove_WithGuardHeldAndBroken` (four tests, one per preset, generated from the preset table)
- `TestAdvance_NothingToSource_PassesSourcingAtOnce`
- `TestAdvance_ReadyWithoutAssembling_ChecksItems` (`pronta_entrega`, `orcamento_primeiro`)
- `TestRecordPayment_DepositConfirmsAndDates_CB500X`
- `TestRecordPayment_RefusesAboveBalance`
- `TestVoidPayment_RemovesFromGuards_KeepsRelease`
- `TestChange_BeforePayment_Free`, `TestChange_AfterPayment_NeedsApproval`, `TestChange_RefusedInAssembling`, `TestChange_LaterForecastMovesPromise`, `TestChange_LowerTotalShowsRefund`, `TestChange_KeepsSurvivingItemState`
- `TestChangeDeliveryDate_OwnerPromiseSurvivesDeposit`
- `TestUseBackupSupplier_RedatesAndSwapsCost`
- `TestCancel_RefusedAfterHandover`
- `TestMessages_NeverNameSupplierOrRisk` (fuzz the facts)
- `TestEvents_CarryNoPersonalData` (JSON walk)

### 1.8 As built in 4.2–4.6

What PR B changed from the sections above. Later steps build against this.

- **`workflow`** (4.2):
  - `ErrInvalidTransition` matches `shared.ErrConflict` (it is a 409).
  - `PhasesEntered` leaves out the phase the order is already in: a move inside one phase enters nothing, and a skip enters every phase it passes.
  - `New` also checks the id, version, key and name.
  - `Rank` is -1 for an unknown phase.
- **`shared.CivilDate`** (4.3):
  - `ErrInvalidDate` matches `ErrInvalid`.
  - An unset date stays unset through `AddDays`, and `ExpectedDate` with no release gives an unset date. `order` always passes a base day: the release date, or today.
  - Negative days clamp at 0.
- **`customer`** (4.4):
  - `Email()` returns `identity.Email`.
  - A repeated address id in a spec is a field error.
  - Notes allow line breaks.
- **`internal/testkit`** holds the four presets (`testkit.Presets`, `testkit.Preset`, `testkit.PresetSpecs`) and `testkit.NewOrder()`, so the order tests and 4.8's integration test share one copy. `domain/workflow` keeps its own, because it cannot import testkit.
  - `.golangci.yml` lets only domain `_test.go` files import testkit.
  - testkit itself may import only the domain.
- **`order`** (4.5, 4.6):
  - `Advance(to, handover, by, today, now)` takes `today`: an order whose deposit due is zero is released on the day it leaves awaiting_deposit.
  - `Terms.Address` is a `*DeliveryAddress` snapshot (id, label, CEP, street, number, complement, district, city, UF). The order keeps its copy, so a customer's address can be edited or removed.
  - `Margin()` returns `catalog.Margin` (from `catalog.MarginOf`), over the total less the freight.
  - `PromiseSetByOwner` is `PromiseChangedByHand`.
  - New: `ErrAlreadyReleased` (a second release, or a release after a paid deposit) and `ErrInvalidRecord` (a broken stored row; never `ErrInvalid`).
  - New accessors and methods: `Order.Record()`, `HandedOver()`, `CancelledAt()`, `Forecast(today)`, `AssemblyDaysCounted()`, `Editable()`, `Closed()`, `Item.NextMoves()`, `Item.ArrivedOn()`, `Line.KitDays()`.
  - A new history kind, `backup_supplier`. History entries carry `Handover`, `PaymentKind` and `PaymentMethod`, and never a supplier name.
  - `TrackingMessage(f, link)` and `CreatedMessage(f, link)` take the link as an argument, and `MessageFacts` has no link field. So `DateChangedMessage` and `ChangedMessage` cannot carry a tracking link (ADR 0030, Amendments).
  - A voided deposit keeps the release date, but the guard counts only payments that are not void, so the order cannot leave awaiting_deposit on it.
- **From the review of PR B:**
  - **The confirmed deposit is frozen.** The deposit due when it was first confirmed is kept, `Record.DepositConfirmedCents`. `DepositDue` returns it, capped at the total. So a higher total or percent after a change adds to the balance, and `order.deposit_confirmed` fires once per confirmation (again only after a void and a new payment).
  - **A voided deposit can be replaced.** The owner may `Release` the order, which counts dates from that day. Or the deposit may be paid again, which releases from the day it is paid. `Release` refuses only an order the owner already released or whose deposit is paid, and refuses a zero user or a zero today.
  - **Item dates on a change.** Kept items with the same timing keep their expected date. New or retimed items count from today once the order is released. Before the release, every item that has not arrived is forecast from today.
  - **No alerts before the release.**
  - **Bounds.** Prices, option prices, freight and costs are bounded by the catalog's R$ 1.000.000.000,00. The order's currency must be valid. The staff-only sourcing (cost, backup supplier, backup lead days) is checked.
  - **Required arguments.** Every move refuses a zero today, and `Cancel` refuses a zero time.
  - **testkit stays in tests.** `.golangci.yml` denies `internal/testkit` in every non-test file.
  - **A backup switch is visible to the client.** `UseBackupSupplier` changes the item's `LeadDays` in the client-visible snapshot. That is intended: ready days are client data (ADR 0030), and no supplier name or cost reaches the snapshot.
- **For 4.7, the orders table:**
  - add `deposit_confirmed_cents bigint CHECK (deposit_confirmed_cents >= 0)`, null before the deposit is confirmed;
  - add `handed_over_as text CHECK (handed_over_as IN ('pickup','delivery'))`, set together with `handed_over_at`;
  - rename `promise_set_by_owner` to `promise_changed_by_hand`;
  - replace the `delivery_address_id` foreign key with a `delivery_address jsonb` snapshot plus a plain `delivery_address_id uuid`, with no foreign key, so removing a customer's address never fails;
  - the history `detail` holds `handover`, `paymentKind` and `paymentMethod`.

---

## 2. Migrations (step 4.7)

Next free numbers on `main` are expected to be `0012`–`0015`; take whatever is free (ADR 0014). Every tenant table follows the `new-migration` skill: `tenant_id uuid NOT NULL REFERENCES tenants ON DELETE CASCADE`, `UNIQUE (tenant_id, id)`, composite FKs, `created_at`, `updated_at`, an index starting with `tenant_id`, `ENABLE` and `FORCE ROW LEVEL SECURITY`, grants to `encomendo_app` only.

### 2.1 `0012_customers.sql`

```sql
CREATE TABLE customers (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    whatsapp_phone text NOT NULL CHECK (whatsapp_phone ~ '^55[1-9][0-9]{9,10}$'),
    email citext,
    notes text NOT NULL DEFAULT '' CHECK (char_length(notes) <= 2000),
    client_account_id uuid REFERENCES client_accounts (id) ON DELETE SET NULL,
    version int NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, whatsapp_phone)
);
CREATE INDEX customers_tenant_name_idx ON customers (tenant_id, lower(name), id);

CREATE TABLE customer_addresses (
    tenant_id uuid NOT NULL,
    id uuid PRIMARY KEY,
    customer_id uuid NOT NULL,
    position int NOT NULL CHECK (position >= 1),
    label text NOT NULL, cep char(8) NOT NULL CHECK (cep ~ '^[0-9]{8}$'),
    street text NOT NULL, number text NOT NULL DEFAULT '', complement text NOT NULL DEFAULT '',
    district text NOT NULL DEFAULT '', city text NOT NULL, state char(2) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, customer_id) REFERENCES customers (tenant_id, id) ON DELETE CASCADE
);
-- tenant_isolation on both; audit triggers on both.
```

### 2.2 `0013_orders.sql`

```sql
CREATE TABLE order_counters (
    tenant_id uuid PRIMARY KEY REFERENCES tenants (id) ON DELETE CASCADE,
    last_number int NOT NULL DEFAULT 0
);

CREATE TABLE orders (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id uuid PRIMARY KEY,
    number int NOT NULL CHECK (number >= 1),
    customer_id uuid NOT NULL,
    workflow_id uuid NOT NULL, workflow_version int NOT NULL,
    status_key text NOT NULL,              -- the last real status; cancelled_at decides "cancelled"
    release_date date, released_by uuid,   -- released_by set only for a release without the deposit
    delivery_date date NOT NULL,
    promise_set_by_owner boolean NOT NULL DEFAULT false,
    assembly_days int NOT NULL CHECK (assembly_days BETWEEN 0 AND 60),
    safety_margin_days int NOT NULL CHECK (safety_margin_days BETWEEN 0 AND 60),
    deposit_percent int NOT NULL CHECK (deposit_percent BETWEEN 0 AND 100),
    currency char(3) NOT NULL,
    subtotal_cents bigint NOT NULL CHECK (subtotal_cents >= 0),
    freight_cents bigint NOT NULL DEFAULT 0 CHECK (freight_cents >= 0),
    discount_cents bigint NOT NULL DEFAULT 0 CHECK (discount_cents >= 0),
    handover text NOT NULL CHECK (handover IN ('pickup', 'delivery')),
    delivery_address_id uuid,
    handed_over_at timestamptz,
    cancelled_at timestamptz, cancel_reason text,
    tracking_token_hash bytea NOT NULL UNIQUE CHECK (octet_length(tracking_token_hash) = 32),
    client_account_id uuid REFERENCES client_accounts (id) ON DELETE SET NULL,
    created_by uuid,
    version int NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, number),
    FOREIGN KEY (tenant_id, customer_id) REFERENCES customers (tenant_id, id),
    FOREIGN KEY (tenant_id, delivery_address_id) REFERENCES customer_addresses (tenant_id, id),
    FOREIGN KEY (workflow_id, workflow_version, status_key) REFERENCES workflow_statuses (workflow_id, workflow_version, key),
    CHECK ((cancelled_at IS NULL) = (cancel_reason IS NULL)),
    CHECK (handover = 'pickup' OR delivery_address_id IS NOT NULL)
);
CREATE INDEX orders_tenant_active_idx ON orders (tenant_id, delivery_date, id) WHERE cancelled_at IS NULL AND handed_over_at IS NULL;
CREATE INDEX orders_tenant_customer_idx ON orders (tenant_id, customer_id, created_at DESC);
CREATE INDEX orders_client_idx ON orders (client_account_id) WHERE client_account_id IS NOT NULL;

CREATE TABLE order_lines (
    tenant_id uuid NOT NULL, id uuid PRIMARY KEY, order_id uuid NOT NULL,
    position int NOT NULL,
    kit_id uuid, kit jsonb,                 -- KitSnapshot; both null for a loose item
    price_cents bigint NOT NULL CHECK (price_cents >= 0),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id) ON DELETE CASCADE,
    CHECK ((kit_id IS NULL) = (kit IS NULL))
);

CREATE TABLE order_items (
    tenant_id uuid NOT NULL, id uuid PRIMARY KEY, order_id uuid NOT NULL, line_id uuid NOT NULL,
    position int NOT NULL,
    snapshot jsonb NOT NULL,                -- ItemSnapshot: name, unit price, options, lead days
    sourcing_mode text NOT NULL CHECK (sourcing_mode IN ('to_order', 'stock', 'in_house')),
    quantity int NOT NULL CHECK (quantity BETWEEN 1 AND 99),
    status text NOT NULL, check_result text CHECK (check_result IN ('ok', 'defective')),
    expected_date date NOT NULL, arrived_on date,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, line_id) REFERENCES order_lines (tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id) ON DELETE CASCADE
);

-- Staff-only (ADR 0024, ADR 0030): what a client must never read.
CREATE TABLE order_item_sourcing (
    tenant_id uuid NOT NULL, order_item_id uuid PRIMARY KEY,
    supplier_id uuid, supplier_name text NOT NULL DEFAULT '',
    cost_cents bigint CHECK (cost_cents >= 0),
    backup_supplier_id uuid, backup_supplier_name text, backup_lead_days int, backup_cost_cents bigint,
    FOREIGN KEY (tenant_id, order_item_id) REFERENCES order_items (tenant_id, id) ON DELETE CASCADE
);
-- Not a FK to suppliers: the snapshot outlives a renamed or removed supplier.

CREATE TABLE payments (
    tenant_id uuid NOT NULL, id uuid PRIMARY KEY, order_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('deposit', 'balance')),
    amount_cents bigint NOT NULL CHECK (amount_cents > 0), currency char(3) NOT NULL,
    method text NOT NULL CHECK (method IN ('pix', 'dinheiro', 'cartao_credito', 'cartao_debito', 'transferencia', 'outro')),
    recorded_by uuid, recorded_at timestamptz NOT NULL,
    voided_at timestamptz, voided_by uuid, void_reason text,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id) ON DELETE CASCADE,
    CHECK ((voided_at IS NULL) = (void_reason IS NULL))
);
CREATE INDEX payments_tenant_recorded_idx ON payments (tenant_id, recorded_at);

CREATE TABLE order_status_history (
    tenant_id uuid NOT NULL, id uuid PRIMARY KEY, order_id uuid NOT NULL,
    kind text NOT NULL, by_user uuid, at timestamptz NOT NULL,
    detail jsonb NOT NULL,                  -- the HistoryEntry fields for its kind
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX order_status_history_order_idx ON order_status_history (tenant_id, order_id, at);

CREATE TABLE order_messages (
    tenant_id uuid NOT NULL, id uuid PRIMARY KEY, order_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('tracking_link', 'created', 'date_changed', 'changed')),
    to_phone text NOT NULL, text text NOT NULL, url text NOT NULL,
    by_user uuid, at timestamptz NOT NULL,
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id) ON DELETE CASCADE
);
```

Policies:

- `tenant_isolation` on `order_counters`, `orders`, `order_lines`, `order_items`, `payments`, `order_status_history`, `order_messages`.
- `staff_only` on `order_item_sourcing`, and no other policy.
- `client_read` (`FOR SELECT`) on `orders`: `current_tenant_id() IS NULL AND client_account_id IS NOT NULL AND client_account_id = current_client_account_id()`; on `order_lines`, `order_items` and `payments`: the same through `EXISTS (SELECT 1 FROM orders o WHERE o.tenant_id = <t>.tenant_id AND o.id = <t>.order_id AND <the orders condition>)`. `order_messages` hold the client's phone and WhatsApp text, and history holds staff names, so neither gets a client policy.
- `order_messages` never holds a tracking token (security review of 4.1, D1): the use case records the text with the tracking URL replaced by `[link de acompanhamento]` and `url` is the `wa.me` URL built from that redacted text, so the plain token exists only in the HTTP answer that made it. Reads are still staff-only (`staff_only` instead of `tenant_isolation`): the texts carry the client's phone. Tests: `TestSendTrackingLink_StoredMessageHasNoToken` (app), and `TestMessages_DateAndChangeCarryNoTrackingLink` (domain: `DateChangedMessage` and `ChangedMessage` never contain `/p/`; only `TrackingMessage` and `CreatedMessage` take a `TrackingURL`).
- Audit triggers on `orders`, `order_lines`, `order_items` and `payments`. Not on `order_item_sourcing` (costs) and not on `order_messages` (the client's phone and texts).
- The `tracking_token_hash` column is never in `audit_log` either: `0005`'s `audit_row()` takes no arguments, so this migration adds `audit_row_redacted()`, the same function with `to_jsonb(OLD) - 'tracking_token_hash'` and `to_jsonb(NEW) - 'tracking_token_hash'`, and the `orders` trigger uses it.
- Grants follow the column-grant pattern of `0002`: `SELECT, INSERT` on every table, `UPDATE` only on the columns a use case changes (never `tenant_id`, `id`, `number`, `created_at`, `created_by`), `DELETE` only on `order_lines`, `order_items`, `order_item_sourcing` and `customer_addresses` (replaced as sets). `payments`, history and messages get no `DELETE`.

### 2.3 `0014_idempotency.sql`

```sql
CREATE TABLE idempotency_keys (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    key text NOT NULL CHECK (char_length(key) BETWEEN 1 AND 128),
    request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
    resource_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    PRIMARY KEY (tenant_id, key)
);
-- tenant_isolation; no audit.
```

The stored "response" is the id of the resource the first call created; a replay answers with that resource as it is now and the first call's status (ADR 0030, amended in this design: an id outlives schema changes that a stored body would not).

### 2.4 `0015_settings_time_zone.sql`

```sql
ALTER TABLE tenant_settings ADD COLUMN time_zone text NOT NULL DEFAULT 'America/Sao_Paulo'
    CHECK (time_zone IN ('America/Noronha', 'America/Sao_Paulo', 'America/Manaus', 'America/Rio_Branco'));
```

`GRANT UPDATE (time_zone) ON tenant_settings TO encomendo_app;`, next to the column grants `0007` gives the other settings.

### 2.4a As built in 4.7

What step 4.7 changed after the security review:

- **History is the 0010 pair.** `order_status_history` has `history_write` (INSERT, checked against the tenant) and `staff_read` (SELECT, needs a staff user), as `audit_log` does. A store-page scope may write the entry its order creates, but reads none. Repositories must insert history without `RETURNING`, because that would need the read policy.
- **`client_account_id` cannot be inserted or updated.** The INSERT grants on `orders` and `customers` are column lists without it. So 4.9's and 4.12's GORM models must leave it out on writes: tag it `<-:false` or `Omit` it. M7 adds a reviewed linking path.
- **An item's line belongs to the item's own order.** The foreign key is `(tenant_id, order_id, line_id)`, against a new `UNIQUE (tenant_id, order_id, id)` on `order_lines`.
- **`audit_row` already drops every `*_hash` column,** so no `audit_row_redacted()` was needed. A test pins which tables are audited, and another checks that the orders audit rows leave the token hash out.

### 2.5 RLS suite

- `ordersTables` (new list): every table above enters `tenantTables`.
- `order_item_sourcing` and `order_messages` enter the staff-only list (the sixth case).
- New `clientReadTables = []string{"orders", "order_lines", "order_items", "payments"}` with three cases, `TestClientRead_OwnOrdersAcrossStores`, `TestClientRead_NotAnotherClients`, `TestClientRead_WritesRefused`, over two tenants and two client accounts.
- `TestClientRead_InternalTablesEmpty`: as a client, `order_item_sourcing`, `order_messages`, `order_status_history`, `customers`, `customer_addresses` return zero rows.
- `TestPolicies_ClientReadOnlyWhereListed`: the catalog query that lists every policy besides `tenant_isolation` and `staff_only` returns exactly `client_read` on the four tables.

---

## 3. Application layer

### 3.1 `app/shared` additions (step 4.10)

`idempotency.go`:

```go
type IdempotencyKey struct {
	Key         string   // the header, 1..128 printable ASCII
	RequestHash [32]byte // SHA-256 of method, path and body, from the HTTP adapter
}

func (k IdempotencyKey) IsZero() bool

// Idempotency remembers which resource a key created, inside the write's own transaction.
type Idempotency interface {
	// Recall returns the resource id a key created, ok false when the key is new. It returns
	// ErrIdempotencyKeyReused when the key was used with another request hash.
	Recall(ctx context.Context, k IdempotencyKey) (resource string, ok bool, err error)
	// Remember stores the key; it must run in the same WithinTx as the write.
	Remember(ctx context.Context, k IdempotencyKey, resource string, ttl time.Duration) error
}

var ErrIdempotencyKeyReused = errors.New("idempotency key reused") // matches shared.ErrConflict
```

`sharedfake.Idempotency` with `OnRollback` undo; `RunIdempotencyContract` in `test/contract/sharedports`.

### 3.2 `app/workflow` (step 4.8)

```go
type View struct {
	ID       shared.WorkflowID
	Version  int
	Key      string
	Name     string
	Statuses []StatusView // key, label, clientLabel, phase
}

type UseCases interface {
	Presets(ctx context.Context) ([]View, error) // settings:read
}

// Source is what other contexts (tenancy, order) use through inproc ports; no authorization,
// because the data is global and read-only.
type Source interface {
	Get(ctx context.Context, id shared.WorkflowID, version int) (workflow.Workflow, error) // shared.ErrNotFound
	Default(ctx context.Context) (workflow.Workflow, error)
}

type Repository interface {
	Published(ctx context.Context) ([]workflow.Workflow, error) // latest version of each preset
	ByVersion(ctx context.Context, id shared.WorkflowID, version int) (workflow.Workflow, error)
	Default(ctx context.Context) (workflow.Workflow, error)
}

func NewService(repo Repository, cache appshared.Cache) *Service // implements UseCases and Source
```

Cache keys: `g:workflow:presets` and `g:workflow:{id}:{version}`, 10 minutes. The repository reads outside `WithinTx` scope requirements? No: the global tables need no tenant, but every query still runs through `TxManager` with whatever scope the caller has, so `Service` takes `appshared.TxManager` too.

### 3.3 `app/tenancy` changes (step 4.8)

```go
type ChooseWorkflowInput struct {
	Workflow        shared.WorkflowID
	Version         int
	ExpectedVersion int // the settings version read
}

func (s *Service) ChooseStatusWorkflow(ctx context.Context, in ChooseWorkflowInput) (SettingsView, error) // settings:write; workflow_not_found
```

- `SettingsView` and `UpdateSettingsInput` gain `TimeZone`. An empty `TimeZone` in `UpdateSettingsInput` (an older client that leaves it out) keeps the stored zone, as the contract says; it never falls back to São Paulo. Test: `TestUpdateSettings_OmittedTimeZoneKeepsTheStoredOne`. `SettingsView.Workflow` gains the statuses (the screen previews them).
- The port `WorkflowPresets` (today `DefaultWorkflow`) gains `Exists(ctx, id, version) (tenancy.WorkflowRef, error)`; `inproc.TenancyWorkflows` implements it over `app/workflow.Source`, replacing the M2 adapter.
- `StoreDefaultsView` gains `DepositPercent`, `TimeZone`, `Slug`, `StoreName`, `WhatsAppPhone`, `Workflow tenancy.WorkflowRef`.

### 3.4 `app/customer` (step 4.9)

```go
type View struct {
	ID        shared.CustomerID
	Name, Phone, Email, Notes string
	Addresses []AddressView
	Version   int
}

type Summary struct {
	ID          shared.CustomerID
	Name, Phone string
	Orders      int
	LastOrderAt *time.Time
}

type ListFilter struct{ Query string } // name prefix (lower) or phone digits

type CreateInput struct{ Spec customer.Spec } // address IDs left zero; the service fills them
type UpdateInput struct{ ID shared.CustomerID; Spec customer.Spec; Version int }

type UseCases interface {
	List(ctx context.Context, f ListFilter, p shared.PageRequest) (shared.Page[Summary], error) // order:read
	Get(ctx context.Context, id shared.CustomerID) (View, error)                               // order:read
	Create(ctx context.Context, in CreateInput) (View, error)                                  // order:write; phone_taken
	Update(ctx context.Context, in UpdateInput) (View, error)                                  // customer:write
}

type Repository interface {
	Insert(ctx context.Context, c *customer.Customer) error
	Update(ctx context.Context, c *customer.Customer) error
	ByID(ctx context.Context, tenant shared.TenantID, id shared.CustomerID) (*customer.Customer, error)
	ByPhone(ctx context.Context, tenant shared.TenantID, phone string) (*customer.Customer, error)
	List(ctx context.Context, tenant shared.TenantID, f ListFilter, p shared.PageRequest) (shared.Page[Summary], error)
}

var ErrPhoneTaken = errors.New("customer: whatsapp phone already used") // matches shared.ErrConflict

func NewService(repo Repository, tx appshared.TxManager, ids appshared.IDGenerator, clock appshared.Clock) *Service
```

`Summary.Orders` and `LastOrderAt` come from a `LEFT JOIN LATERAL` on `orders` in the repository.

### 3.5 `app/catalog` addition (step 4.10)

The order's catalog port needs prices, snapshots and the staff-only sourcing in one read:

```go
type OrderLineRequest struct {
	Kit      shared.KitID  // zero for a loose item
	Item     shared.ItemID // the loose item
	Quantity int           // loose items only; a kit line is one kit
	Swaps    []catalog.Swap
	Options  []OptionChoice // for the loose item, or per kit item
}

type OptionChoice struct {
	Item   shared.ItemID
	Option shared.OptionID
	Choice shared.ChoiceID
}

type OrderItem struct {
	Item           shared.ItemID
	Name           string
	SourcingMode   catalog.SourcingMode
	UnitPrice      shared.Money
	Quantity       int
	ReadyDays      int
	Options        []ChosenOption // name, label, extra price, extra days
	MainSupplier   *SupplierFact  // id, name, lead days, cost (cost nil when unknown)
	BackupSupplier *SupplierFact
	Cost           *shared.Money  // in-house cost or main supplier cost
}

type OrderLine struct {
	Kit                            *OrderKit // id, name, swaps with names
	Price                          shared.Money
	Items                          []OrderItem
	AssemblyDays, SafetyMarginDays *int
}

func (s *Service) OrderLines(ctx context.Context, reqs []OrderLineRequest) ([]OrderLine, error)
```

- Requires `order:write` (it reads costs for the snapshot whatever the caller's role; the order views decide who sees them).
- Refuses an archived item or kit (`catalog.ErrItemArchived`, mapped to `item_unavailable`), a missing required option, a choice not in its option, and every kit swap error.
- Line price: kit → `PriceWithSwaps` + Σ option extras × kit item quantity; loose → (unit price + Σ extras) × quantity.
- Not cached (it carries costs).

### 3.6 `app/order` (steps 4.10, 4.11)

Files: `doc.go`, `ports.go`, `inputs.go`, `service.go`, `create.go`, `quote.go`, `change.go`, `payments.go`, `transitions.go`, `items.go`, `dates.go`, `links.go`, `cancel.go`, `views.go`, `tracking_view.go`, `queries.go`, `today.go`, `finance.go`, `fake/`.

**Ports** (`ports.go`):

```go
type UseCases interface {
	Quote(ctx context.Context, in QuoteInput) (QuoteView, error)
	Create(ctx context.Context, in CreateInput) (CreatedView, error)
	Change(ctx context.Context, in ChangeInput) (ChangedView, error)
	RecordPayment(ctx context.Context, in PaymentInput) (OrderView, error)
	VoidPayment(ctx context.Context, in VoidPaymentInput) (OrderView, error)
	Release(ctx context.Context, in VersionInput) (OrderView, error)
	Advance(ctx context.Context, in AdvanceInput) (OrderView, error)
	MoveItem(ctx context.Context, in MoveItemInput) (OrderView, error)
	UseBackupSupplier(ctx context.Context, in ItemVersionInput) (OrderView, error)
	ChangeDeliveryDate(ctx context.Context, in DateInput) (MessagedView, error)
	SendTrackingLink(ctx context.Context, id shared.OrderID) (LinkView, error)
	Cancel(ctx context.Context, in CancelInput) (OrderView, error)

	Get(ctx context.Context, id shared.OrderID) (OrderView, error)
	List(ctx context.Context, f ListFilter, p shared.PageRequest) (shared.Page[SummaryView], error)
	History(ctx context.Context, id shared.OrderID, p shared.PageRequest) (shared.Page[HistoryView], error)
	Today(ctx context.Context) (TodayView, error)
	Finance(ctx context.Context, from, to shared.CivilDate) (FinanceView, error)
}

type Repository interface {
	NextNumber(ctx context.Context, tenant shared.TenantID) (order.Number, error) // UPDATE … RETURNING in the caller's tx
	Insert(ctx context.Context, o *order.Order) error
	Update(ctx context.Context, o *order.Order) error // version check; writes PendingHistory
	ByID(ctx context.Context, tenant shared.TenantID, id shared.OrderID) (order.Record, error)
	List(ctx context.Context, tenant shared.TenantID, f ListFilter, p shared.PageRequest) (shared.Page[order.Record], error)
	Active(ctx context.Context, tenant shared.TenantID, limit int) ([]order.Record, error) // not handed over, not cancelled, by delivery date
	History(ctx context.Context, tenant shared.TenantID, id shared.OrderID, p shared.PageRequest) (shared.Page[HistoryRow], error)
	Payments(ctx context.Context, tenant shared.TenantID, from, to time.Time) ([]PaymentRow, error)
	HandedOver(ctx context.Context, tenant shared.TenantID, from, to time.Time) ([]order.Record, error)
	AddMessage(ctx context.Context, m Message) error
}

type Catalog interface {
	OrderLines(ctx context.Context, reqs []LineRequest) ([]order.LineSpec, error) // ids left zero; the service fills them
}

type Customers interface {
	Get(ctx context.Context, id shared.CustomerID) (CustomerFacts, error) // name, phone, addresses
	Create(ctx context.Context, name, phone string) (CustomerFacts, error)
}

type Workflows interface {
	Current(ctx context.Context) (workflow.Workflow, error)
	Get(ctx context.Context, id shared.WorkflowID, version int) (workflow.Workflow, error)
}

type Store interface {
	Facts(ctx context.Context) (StoreFacts, error)
}

type StoreFacts struct {
	Name, Slug, WhatsApp           string
	Currency                       string
	DepositPercent                 int
	AssemblyDays, SafetyMarginDays int
	Location                       *time.Location
}

type TrackingURLs interface {
	URL(slug string, token shared.Secret) (string, error)
}

type WhatsApp interface {
	Link(phone, text string) (string, error) // wa.me URL
}
```

`inproc` adapters: `OrderCatalog` (over `app/catalog.OrderLines`, mapping to `order.LineSpec`), `OrderCustomers` (over `app/customer`), `OrderWorkflows` (over tenancy settings + `app/workflow.Source`; this is the architecture's `workflow_source.go`), `OrderStore` (over `app/tenancy.StoreDefaults`).

**Constructor**

```go
type Deps struct {
	Repo        Repository
	Catalog     Catalog
	Customers   Customers
	Workflows   Workflows
	Store       Store
	Tracking    TrackingURLs
	WhatsApp    WhatsApp
	Idempotency appshared.Idempotency
	Secrets     appshared.SecretGenerator
	Tx          appshared.TxManager
	Events      appshared.EventPublisher
	Clock       appshared.Clock
	IDs         appshared.IDGenerator
}

func NewService(d Deps) (*Service, error) // error naming any missing dependency
```

**Inputs** (`inputs.go`):

```go
type LineRequest = appcatalog.OrderLineRequest (mapped, not aliased, to keep the dependency through the port)

type TermsInput struct {
	Handover         order.Handover
	Address          shared.AddressID
	Freight          shared.Money
	Discount         *shared.Money // Create: nil = zero. Change: nil = the order's current value
	DepositPercent   *int          // Create: nil = store default. Change: nil = current
	AssemblyDays     *int          // Create: nil = kit or store default. Change: nil = current
	SafetyMarginDays *int          // as AssemblyDays
	// order:manage is needed when the effective value differs from the
	// baseline: the default on Create and Quote, the order's current value
	// on Change and on a Quote with Order set. So staff can never put an
	// admin's term back to the default by leaving it out (security review
	// of 4.1, B1). Test: TestChange_StaffOmittingTermsKeepsThem,
	// TestChange_StaffResettingDiscountIsForbidden.
}

type NewCustomer struct{ Name, Phone string }

type QuoteInput struct {
	Lines []LineRequest
	Terms TermsInput
	Order shared.OrderID // set when quoting a change: dates count from its release date
}

type CreateInput struct {
	Customer    shared.CustomerID // or New
	New         *NewCustomer      // found by phone first; created when absent
	Lines       []LineRequest
	Terms       TermsInput
	SendToCustomer bool           // "Salvar e enviar ao cliente": advance to the next status and return the wa.me link
	Idempotency appshared.IdempotencyKey // required
}

type ChangeInput struct {
	Order            shared.OrderID
	Lines            []LineRequest
	Terms            TermsInput
	CustomerApproved bool
	Version          int
}

type PaymentInput struct {
	Order       shared.OrderID
	Kind        order.PaymentKind
	Amount      shared.Money
	Method      order.PaymentMethod
	Version     int
	Idempotency appshared.IdempotencyKey
}

type VoidPaymentInput struct{ Order shared.OrderID; Payment shared.PaymentID; Reason string; Version int }
type VersionInput struct{ Order shared.OrderID; Version int }
type AdvanceInput struct{ Order shared.OrderID; To workflow.StatusKey; Handover *order.Handover; Version int }
type MoveItemInput struct{ Order shared.OrderID; Item shared.OrderItemID; To order.ItemStatus; Check *order.Check; Version int }
type ItemVersionInput struct{ Order shared.OrderID; Item shared.OrderItemID; Version int }
type DateInput struct{ Order shared.OrderID; Date shared.CivilDate; Reason string; Version int }
type CancelInput struct{ Order shared.OrderID; Reason string; Version int }

type ListFilter struct {
	Status   workflow.StatusKey
	Phase    workflow.Phase
	Scope    ListScope // "active" (default), "handed_over", "cancelled", "all"
	Customer shared.CustomerID
	Query    string // number ("231", "P-0231") or customer name
	Alert    bool   // only orders with an alert today
}
```

**Authorization** (each use case's first line):

| Use case | Permission | Extra |
| --- | --- | --- |
| Quote, Create | `order:write` | discount, deposit percent or days other than the default need `order:manage` |
| Change | `order:write` before a payment; `order:manage` after | same rule for discount and days |
| RecordPayment, Advance, MoveItem, SendTrackingLink | `order:write` | |
| UseBackupSupplier, ChangeDeliveryDate, Cancel | `order:manage` | |
| Release, VoidPayment | `order:override` | |
| Get, List, History, Today | `order:read` | costs and margins only when `identity.SeesCosts` |
| Finance | `finance:read` | |

**Flow notes**

- `today := shared.DateOf(clock.Now(), facts.Location)` in every write.
- `Create`: `authz.Require(order:write)` first, then idempotency `Recall` → replay `Get` (a replay never skips the permission: `TestCreate_ViewerReplayingAKeyIsForbidden`, and the same for `RecordPayment`; a second call with a key still in flight waits on the row lock and answers `409 conflict`, mapped from the primary-key violation); customer (find by phone, else create); `Workflows.Current`; `Catalog.OrderLines`; fill ids; effective days = input ?? max of kit overrides ?? store; `NextNumber`; `Secrets` → token, `RotateTracking`-equivalent hash in `NewSpec`; `order.New`; `Insert`; when `SendToCustomer`, `Advance` to the first status after draft (no guard applies) and build the created message; `Remember`; publish `Created` (+ `Advanced`); `AddMessage`. Returns `CreatedView{Order, TrackingURL, WhatsAppURL}`. The tracking URL is in this answer only.
- `Change`: after payment, returns `ChangedView{Order, Summary ChangeResult, WhatsAppURL}` with `ChangedMessage`; before payment, `WhatsAppURL` is empty.
- `RecordPayment`: idempotency like `Create`, resource = payment id; replay returns the order.
- `ChangeDeliveryDate`: returns `MessagedView{Order, WhatsAppURL}` with `DateChangedMessage`; `AddMessage`. Nothing else leaves the server.
- `SendTrackingLink`: new secret, `RotateTracking`, `Update`, `TrackingMessage`, `AddMessage`; returns `LinkView{TrackingURL, WhatsAppURL}`.
- Every write: `Update` with the version, events in the same transaction, no cache.

**Views** (`views.go`, `tracking_view.go`):

```go
type StatusView struct{ Key, Label, ClientLabel string; Phase workflow.Phase }

type ItemView struct {
	ID           shared.OrderItemID
	Line         shared.OrderLineID
	Name         string
	SourcingMode catalog.SourcingMode
	Quantity     int
	UnitPrice    shared.Money
	Options      []OptionView
	Status       order.ItemStatus
	Check        *order.Check
	Expected     shared.CivilDate
	ArrivedOn    *shared.CivilDate
	Alerts       []schedule.Alert
	NextMoves    []order.ItemStatus  // what MoveItem accepts now
	Supplier     *SupplierView       // name and lead days; staff roles (not a client view)
	Backup       *SupplierView       // when a switch is possible
	Cost         *shared.Money       // owner only
}

type OrderView struct {
	ID, Number         string // Number "P-0231"
	Customer           CustomerRef // id, name, phone
	Status             StatusView
	Workflow           WorkflowRef // id, version, name, statuses
	Moves              []StatusView // what Advance accepts now
	Lines              []LineView
	Items              []ItemView
	Terms              TermsView
	Subtotal, Freight, Discount, Total, DepositDue, Paid, BalanceDue, RefundDue shared.Money
	DepositPaid, Released bool
	Payments           []PaymentView
	ReleaseDate        *shared.CivilDate
	DeliveryDate       shared.CivilDate
	ArrivalLimit       shared.CivilDate
	Editable           Editability // "free", "approval", "closed"
	CreatedAt          time.Time
	Cost               *shared.Money // owner only
	Margin             *MarginView   // owner only
	Version            int
}

// TrackingView is the client's view (M7 serves it). It has no field that could carry a
// supplier, a cost, a margin, an alert or a staff name: it is a separate type, never
// OrderView with fields left out.
type TrackingView struct {
	Store        string
	Number       string
	Steps        []TrackingStep // client labels in order, Current marks the order's status; cancelled shows one step
	DeliveryDate shared.CivilDate // the promise (orders.delivery_date), never the forecast
	Items        []TrackingItem // name, quantity, options as words, "pronto" or "a caminho" from Arrived
	Total, Paid, BalanceDue shared.Money
	Handover     order.Handover
}

func NewTrackingView(o *order.Order, store string) TrackingView
```

**Queries** (`queries.go`, `today.go`, `finance.go`):

- `List` returns `SummaryView{ID, Number, Customer, Status, ItemsReady, ItemsTotal, DeliveryDate, Total, Alerts []schedule.Alert}`; the `Alert` filter loads the page and filters by `o.Alerts(today)` (active orders only; the repository caps a page at 100).
- `Today` reads `Active(limit 500)` once and builds:
  - `Counts{Active, AwaitingDeposit, WithAlert, DueIn7Days}`;
  - `NeedsYou []NeedView{Kind, Order SummaryView, Item string, Days int, Amount *shared.Money}`, kinds in this order: `late`, `at_risk`, `awaiting_deposit` (more than 3 days in the `awaiting_deposit` phase), `ready_not_handed_over` (more than 2 days in `ready`), `refund_due`;
  - `Upcoming []SummaryView` (by delivery date, 20).
  The thresholds are constants in `today.go` (`awaitingDepositDays = 3`, `readyDays = 2`).
- `Finance(from, to)`: `Received{Total, ByKind, ByMethod}` from non-void payments in the period (in the store zone); `ToReceive{Total, Orders}` from active orders with a balance; `ToRefund []…`; `Margins []MarginRow` from orders handed over in the period, `Margin` nil without a full cost. `from` after `to` or a period over 366 days → `ErrInvalid`.

### 3.7 `app/schedule` (step 4.11)

```go
type Calendar interface {
	Today(ctx context.Context) (shared.CivilDate, *time.Location, error) // the store's day
}

type Service struct{ /* store app/order.Store port, clock, orders port */ }

func (s *Service) Today(ctx context.Context) (shared.CivilDate, *time.Location, error)
func (s *Service) Recalculate(ctx context.Context, id shared.OrderID) error // order:write; re-derives expected dates from the snapshot and the release date, for M5's promised dates and after a time-zone change
```

`app/order` uses its own `Store` port for the zone; `app/schedule` exists for M5 and M6 (`RecordPromisedDate` calls `Recalculate`; the worker's `CheckAll` lands in M6, not stubbed now).

---

## 4. Adapters

### 4.1 Postgres (steps 4.8, 4.9, 4.12, 4.13)

| File | Implements | Notes |
| --- | --- | --- |
| `workflow_repo.go`, `models/workflow.go` | `appworkflow.Repository` | `Published` = the highest version per id; `Default` by `is_default` |
| `customer_repo.go`, `models/customer.go` | `appcustomer.Repository` | addresses replaced as a set (delete missing ids, upsert others); `23505` on the phone index → `ErrPhoneTaken` |
| `order_repo.go`, `order_queries.go`, `models/order.go` | `apporder.Repository` | below |
| `idempotency_repo.go` | `appshared.Idempotency` | `SELECT … FOR UPDATE` on `(tenant_id, key)` where `expires_at > now()`; insert with `ON CONFLICT DO NOTHING` and re-read, so two concurrent first calls end with one winner and one `version_conflict`-free replay |
| `tenancy_repo.go` | settings `time_zone` | column added to the model and the `Select` list |

`order_repo.go`:

- `NextNumber`: `INSERT INTO order_counters (tenant_id) VALUES (?) ON CONFLICT DO NOTHING`, then `UPDATE order_counters SET last_number = last_number + 1 WHERE tenant_id = ? RETURNING last_number`.
- `Insert`/`Update`: the order row; lines and items replaced as a set by id (delete removed ids, upsert others); `order_item_sourcing` written in its own statements (a non-staff scope would fail them, and only staff scopes write orders in M4); new payments inserted, voids updated; `PendingHistory` appended. Version check on `orders` (`WHERE version = ?`, `RowsAffected == 0` → `ErrVersionConflict`).
- `ByID`: the order, lines, items, sourcing (zero rows outside a staff scope is fine: `Sourcing` stays empty), payments.
- `List`: keyset on `(delivery_date, id)` for active and `(created_at DESC, id)` for the rest; `Query` matches `number = ?` when it parses as a number, else `lower(c.name) LIKE lower(?) || '%'`.
- `Payments`, `HandedOver`: `Raw` with bound parameters.

Models: `models/order.go` with `Order`, `OrderLine`, `OrderItem`, `OrderItemSourcing`, `Payment`, `HistoryRow`, `Message`, `toDomain`/`fromDomain`. Snapshots marshal through small JSON structs in the model file, never the domain types directly.

### 4.2 `adapter/auth/tracking.go` (step 4.13)

```go
type TrackingURLs struct{ template string } // validated at construction

func NewTrackingURLs(template string) (*TrackingURLs, error) // must contain "{slug}" once, parse as a URL with scheme http or https
func (t *TrackingURLs) URL(slug string, token shared.Secret) (string, error) // template with slug + "/p/" + token.Reveal()
```

The lookup by token for M7 (`ByToken`, constant-time on the hash through the unique index) is **not** built now; M7 adds it with the store route.

### 4.3 `adapter/notify/whatsapp_link.go` (step 4.13)

```go
type WhatsAppLinks struct{}

func (WhatsAppLinks) Link(phone, text string) (string, error)
// "https://wa.me/{digits}?text={url.QueryEscape(text)}"; ErrInvalid unless shared.ValidWhatsAppPhone(phone); text ≤ 2000 runes.
```

### 4.4 Configuration (step 4.14)

`platform/config`: `STORE_ORIGIN_TEMPLATE` (required), checked like `ADMIN_ORIGIN`: `https` outside `development` and `test`. Defaults written into `deploy/docker-compose.yml` (`http://{slug}.localhost:${STORE_PORT}`), `deploy/docker-compose.prod.yml` and `deploy/prod.env.example` (`https://{slug}.encomendo.com.br`). `cmd/api/main.go` imports `_ "time/tzdata"`.

### 4.5 `inproc` (steps 4.8, 4.10)

`workflow_source.go` (`OrderWorkflows`, `TenancyWorkflows`), `order_catalog.go`, `order_customers.go`, `order_store.go`, each with a compile-time assertion and a test against fakes.

---

## 5. HTTP (steps 4.1, 4.14)

### 5.1 Routes

All under `/api/v1/admin`, behind authn and the suspended-store check, as the M3 routes.

| Method and path | Handler | Use case | Status |
| --- | --- | --- | --- |
| `GET /status-workflows` | `Workflows.List` | `Presets` | 200 |
| `PUT /settings/status-workflow` | `Tenancy.ChooseWorkflow` | `ChooseStatusWorkflow` | 200 |
| `GET /customers` | `Customers.List` | `List` | 200 |
| `POST /customers` | `Customers.Create` | `Create` | 201 |
| `GET /customers/{customerId}` | `Customers.Get` | `Get` | 200 |
| `PUT /customers/{customerId}` | `Customers.Update` | `Update` | 200 |
| `POST /orders/quote` | `Orders.Quote` | `Quote` | 200 |
| `POST /orders` | `Orders.Create` | `Create` | 201 (replay 201) |
| `GET /orders` | `Orders.List` | `List` | 200 |
| `GET /orders/{orderId}` | `Orders.Get` | `Get` | 200 |
| `PUT /orders/{orderId}` | `Orders.Change` | `Change` | 200 |
| `POST /orders/{orderId}/advance` | `Orders.Advance` | `Advance` | 200 |
| `POST /orders/{orderId}/release` | `Orders.Release` | `Release` | 200 |
| `POST /orders/{orderId}/items/{itemId}/move` | `Orders.MoveItem` | `MoveItem` | 200 |
| `POST /orders/{orderId}/items/{itemId}/use-backup-supplier` | `Orders.UseBackupSupplier` | `UseBackupSupplier` | 200 |
| `POST /orders/{orderId}/payments` | `Orders.RecordPayment` | `RecordPayment` | 201 (replay 201) |
| `POST /orders/{orderId}/payments/{paymentId}/void` | `Orders.VoidPayment` | `VoidPayment` | 200 |
| `PUT /orders/{orderId}/delivery-date` | `Orders.ChangeDeliveryDate` | `ChangeDeliveryDate` | 200 |
| `POST /orders/{orderId}/tracking-link` | `Orders.SendTrackingLink` | `SendTrackingLink` | 200 |
| `POST /orders/{orderId}/cancel` | `Orders.Cancel` | `Cancel` | 200 |
| `GET /orders/{orderId}/history` | `Orders.History` | `History` | 200 |
| `GET /today` | `Today.Get` | `Today` | 200 |
| `GET /finance/summary` | `Finance.Summary` | `Finance` | 200 |

`GET /settings` and `PUT /settings` keep their paths and gain `timeZone`; the settings response gains the workflow's statuses. (As built in 4.1: the settings route is `PUT`, as since M2.)

`Idempotency-Key` is read by a helper in `handler/idempotency.go`: `idempotencyKey(r, body []byte) (appshared.IdempotencyKey, error)`, hashing `method + "\n" + path + "\n" + body`. Missing on `POST /orders` or `POST /orders/{id}/payments` → `idempotency_key_required`. The body limit stays as today.

### 5.2 DTOs

`dto/workflow.go`, `dto/customer.go`, `dto/order_requests.go`, `dto/order_responses.go`, `dto/today.go`, `dto/finance.go`. Money is `{cents, currency}`; dates are `"YYYY-MM-DD"`; ids are strings; owner-only fields are pointers with `omitempty`. `dto.NewTrackingOrder` exists for the contract test only until M7.

### 5.3 Problem codes (`respond/problem.go`)

| Code | Status | Title (pt-BR) | From |
| --- | --- | --- | --- |
| `invalid_transition` | 409 | Este pedido não pode ir para este status. | `order.ErrInvalidTransition` |
| `guard_failed` | 409 | (by guard, below) + `guard` field | `*order.GuardError` |
| — `deposit_unpaid` | | O sinal ainda não foi pago. Registre o sinal ou libere o pedido sem ele. | |
| — `items_not_ready` | | Nem todos os itens chegaram e foram conferidos. | |
| — `balance_unpaid` | | Falta receber o saldo deste pedido. | |
| `order_not_editable` | 409 | Este pedido já está em montagem e não pode mais ser alterado. | `order.ErrNotEditable` |
| `order_closed` | 409 | Este pedido já foi entregue ou cancelado. | `order.ErrOrderClosed` |
| `approval_required` | 400 | Confirme que o cliente aprovou a mudança. | `order.ErrApprovalRequired` |
| `payment_exceeds_due` | 400 | O valor passa do que falta pagar. | `order.ErrPaymentExceedsDue` |
| `invalid_item_move` | 409 | Este item não pode ir para esta situação. | `order.ErrInvalidItemMove` |
| `no_backup_supplier` | 409 | Este item não tem fornecedor reserva ou já foi comprado. | `order.ErrNoBackupSupplier` |
| `item_unavailable` | 409 | Um item ou kit do pedido está arquivado. | `catalog.ErrItemArchived` via `OrderLines` |
| `phone_taken` | 409 | Já existe um cliente com este WhatsApp. | `appcustomer.ErrPhoneTaken` |
| `workflow_not_found` | 404 | Esta lista de status não existe. | `tenancy.ErrWorkflowNotFound` |
| `idempotency_key_required` | 400 | Falta o cabeçalho Idempotency-Key. | handler |
| `idempotency_key_reused` | 422 | Esta chave já foi usada em outro pedido. | `appshared.ErrIdempotencyKeyReused` |

`ErrDateInPast` and `ErrHandoverRequired` are `ValidationError`s with fields (`date`, `handover`), so they come out as `invalid` with the field messages of the design document. The mapping test covers every row, and the codes enter `openapi.yaml`'s `ProblemCode` enum in 4.1.

### 5.4 Tests

- `order_routes_test.go`: every route, its method (405 otherwise), authn, suspended store.
- `order_handler_test.go`: decode errors, `Idempotency-Key` missing and replayed, each problem code.
- `order_visibility_test.go`: for owner, admin, staff and viewer, the JSON of `GET /orders/{id}`, `GET /orders` and **every operation that returns an `Order`** (create and its replay, change, advance, release, move item, use backup supplier, record payment and its replay, void payment, delivery date, cancel) has `cost`, `margin` and `items[].cost` only for the owner, and `GET /today` has none for anyone. `app/order` builds every `OrderView` through one function, `toView(o, by shared.Principal, today)`, which applies `identity.SeesCosts`; no other constructor exists; `TestTrackingOrder_HasNoInternalField` walks `dto.NewTrackingOrder` against a refuse-list (`cost`, `margin`, `supplier`, `supplierName`, `leadDays`, `atRisk`, `alerts`, `recordedBy`, `history`).
- Contract test: every response validated against `openapi.yaml`, as M3 does.
- `test/e2e/orders_test.go`: owner signs up → chooses `encomenda_montagem` → item with a supplier (15 days) → customer → order (deposit 50%) → deposit → items moved to received ok → advance to Em montagem → Pronto → balance → Entregue (pickup); a second order: change the date → response has a `wa.me` URL and no message was sent (no SMTP, no outbound call: Mailpit is empty and the API has no HTTP client to call).

---

## 6. OpenAPI (step 4.1)

New tags: `workflows`, `customers`, `orders`, `today`, `finance`. New schemas: `CivilDate`, `Phase`, `OrderStatus`, `StatusWorkflow`, `ChooseWorkflowRequest`, `TimeZone`, `Customer`, `CustomerInput`, `CustomerSummary`, `Address`, `LineRequest`, `OptionChoice`, `Swap`, `TermsInput`, `QuoteRequest`, `Quote`, `CreateOrderRequest`, `CreatedOrder`, `ChangeOrderRequest`, `ChangedOrder`, `Order`, `OrderLine`, `OrderItem`, `ItemStatus`, `Alert`, `Payment`, `PaymentRequest`, `VoidPaymentRequest`, `AdvanceRequest`, `MoveItemRequest`, `DeliveryDateRequest`, `MessagedOrder`, `TrackingLink`, `CancelRequest`, `OrderSummary`, `HistoryEntry`, `Today`, `FinanceSummary`, `TrackingOrder` (marked `x-served-from: M7`). `EditableSettings` gains `timeZone`; `Settings.workflow` gains `statuses`. The `Idempotency-Key` header is a reusable parameter, required on the two POSTs. `ProblemCode` gains the codes of 5.3. `Permission` gains `order:manage`, `order:override`, `customer:write`.

As built in 4.1:

- **Optional until served.** `timeZone` in `EditableSettings` and `statuses` in `StatusWorkflowRef` are not required yet, so today's settings responses and requests still match the contract. Step 4.8 serves both and makes them required.
- **Every new operation is marked `x-unserved: "4.14"`**, so the e2e suite expects `404` until 4.14 serves it and removes the mark.
- **Extra schemas** the listed ones needed: `StatusKey`, `StatusWorkflowList`, `CustomerPhone`, `UF`, `AddressInput`, `ReplaceCustomerRequest`, `CustomerPage`, `NewCustomer`, `Handover`, `Check`, `AlertKind`, `PaymentKind`, `PaymentMethod`, `Actor`, `ChosenOption`, `QuoteLine`, `QuoteItem`, `OrderChange`, `KitRef`, `ItemSupplier`, `Editability`, `OrderPage`, `HistoryKind`, `HistoryPage`, `NeedKind`, `Need`, `MoneyByKey`; responses `WorkflowNotFound`, `CustomerConflict`, `OrderInvalid`, `OrderConflict`, `IdempotencyKeyReused`.
- **Request money is `…Cents`** (`freightCents`, `discountCents`, `amountCents`), as the catalog requests are; responses carry `Money`.
- **`Swap` is `{itemId, withItemId}`**, after `catalog.Swap{Item, With}`. `PaymentRequest` carries `version`, as `PaymentInput` does. `DeliveryDateRequest.reason` is optional.
- **`TrackingOrder`** is closed (`additionalProperties: false`) at every level, with `cancelled` and, per item, `arrived` (the page says "Pronto" or "A caminho").

---

## 7. Web

### 7.1 `packages/api` (step 4.16)

| File | Functions |
| --- | --- |
| `workflows.ts` | `listStatusWorkflows(api)` |
| `settings.ts` | `getSettings`, `updateSettings`, `chooseStatusWorkflow(api, {workflowId, version, expectedVersion})` (moves the settings calls out of `catalog` if they live there; re-export for compatibility) |
| `customers.ts` | `listCustomers(api, {q, cursor, limit})`, `getCustomer`, `createCustomer`, `updateCustomer` |
| `orders.ts` | `quoteOrder`, `createOrder(api, body, idempotencyKey)`, `listOrders(api, filter)`, `getOrder`, `changeOrder`, `advanceOrder`, `releaseOrder`, `moveOrderItem`, `useBackupSupplier`, `recordPayment(api, id, body, idempotencyKey)`, `voidPayment`, `changeDeliveryDate`, `sendTrackingLink`, `cancelOrder`, `orderHistory`, `getToday`, `getFinanceSummary` |
| `idempotency.ts` | `newIdempotencyKey()` (`crypto.randomUUID()`), used by the stores: one key per user action, kept for its retries |

All types from `types/` (generated). No arithmetic on money or dates.

### 7.2 Admin features

```text
web/apps/admin/src/features/
├── settings/      views/SettingsView.vue; components/WorkflowChoice.vue, WorkflowPreview.vue, DefaultsForm.vue; store.ts
├── clientes/      views/ClientesView.vue; components/CustomerTable.vue, CustomerForm.vue, AddressFields.vue, CustomerOrders.vue; store.ts
├── pedidos/       views/PedidosView.vue, OrderFormView.vue (new and change), OrderView.vue;
│                  components/OrderTable.vue, StatusTabs.vue, AlertBadge.vue, CustomerPicker.vue, LineEditor.vue,
│                  KitLine.vue, ItemLine.vue, OptionPicker.vue, TermsFields.vue, QuoteSummary.vue, ChangeSummary.vue,
│                  OrderHeader.vue, DelayBanner.vue, DeadlineTimeline.vue, ItemsTable.vue, ItemMoveMenu.vue,
│                  PaymentsPanel.vue, PaymentDialog.vue, VoidDialog.vue, AdvanceButtons.vue, HandoverDialog.vue,
│                  DateDialog.vue, CancelDialog.vue, TrackingLinkButton.vue, OrderHistory.vue;
│                  store.ts (list), orderStore.ts (one order), formStore.ts (new/change with debounced quote), labels.ts (item status words)
├── hoje/          views/HojeView.vue (rewritten); components/SummaryCards.vue, NeedsYou.vue, UpcomingList.vue; store.ts
└── financeiro/    views/FinanceiroView.vue; components/PeriodPicker.vue, ReceivedPanel.vue, ToReceiveTable.vue, MarginsTable.vue; store.ts
```

Routes (`router/index.ts`), all `requiresAuth` and `requiresStore`:

| Path | Name | Permission |
| --- | --- | --- |
| `/pedidos` | `orders` | `order:read` |
| `/pedidos/novo` | `order-new` | `order:write` |
| `/pedidos/:orderId` | `order` | `order:read` |
| `/pedidos/:orderId/alterar` | `order-change` | `order:write` (the view asks for `order:manage` when the order is paid) |
| `/clientes` | `customers` | `order:read` |
| `/clientes/:customerId` | `customer` | `order:read` |
| `/financeiro` | `finance` | `finance:read` |
| `/configuracoes` | `settings` | `settings:write` |

Menu (`DashboardLayout.vue`): Hoje, Pedidos (`order:read`), Catálogo, Clientes (`order:read`), Financeiro (`finance:read`), Equipe, Configurações (`settings:write`), in that order. Compras, Produção and Estoque are not added until their milestones.

Rules that the reviewers will check:

- The quote is the only source of totals, deposit and dates on the form: `formStore` calls `quoteOrder` 300 ms after the last edit and shows "Calculando…" meanwhile; a stale answer (older request id) is dropped.
- Status badges: `StatusBadge` with the API's label and `toneForPhase(phase)`. Alerts: `AlertBadge` ("Pode atrasar", "Atrasada"), on top of the status, icon + word.
- Buttons that need `order:manage` or `order:override` are hidden by `usePermission`, and the API is still the authority.
- `wa.me` links come from the API and are opened with `window.open(url, '_blank', 'noopener')`; the Android shell opens them outside the app (ADR 0029).
- Every screen has its phone layout (cards, sheets, `enc-sheet-actions`), and `DeadlineTimeline` becomes a vertical list of items with their dates at phone width.

### 7.3 Web tests

- MSW fixtures in `apps/admin/src/testing/orders.ts` with two workflows (`encomenda_montagem`, `pronta_entrega`), an owner and an admin view of the same order, and every problem code of 5.3.
- Per feature: `settings.test.ts`, `clientes.test.ts`, `pedidos.test.ts`, `orderForm.test.ts`, `order.test.ts`, `hoje.test.ts`, `financeiro.test.ts`, and `phone.test.ts` per feature as Catálogo has.
- Named cases worth listing: `shows labels from the order's workflow for two different workflows`, `never computes a total on the client` (the summary shows only what MSW returned), `hides cost and margin for an admin`, `asks for customer approval when changing a paid order`, `keeps the idempotency key on retry`, `opens the wa.me link the API returned`, `shows each guard message`.
- Playwright (4.23): `e2e/pedidos.spec.ts` and `e2e/phone-pedidos.spec.ts` (the flow of 5.4's e2e through the UI, plus Configurações and Clientes).

---

## 8. Hand-offs written into the code

- `app/order.Cancel` has a comment `// M5: cancel unsent purchase lines and flag sent ones (ADR 0030).` and `// M6: release stock reservations.`, with the issue numbers once opened (`TODO(#n)` only with an issue, per `CLAUDE.md`). The session opens the two issues when it reaches 4.10.
- M7 store-page orders run in a scope with no staff user, so `order_item_sourcing` can't be written there. M7 must decide (ADR) between filling the snapshot on the first staff touch and a narrow platform path; 4.10 states this in `app/order/doc.go`.
- `schedule.Timing.Promised` is filled by M5; `InitialStatus(stock)` changes in M6.
- From the security review of 4.7, for M7:
  - **Deleting a client account fails** while an order or customer links to it. The `ON DELETE SET NULL` fires the audit trigger with no tenant set, and `audit_write` refuses it. Unlink in each store's own scope first, or delete through the platform role. Never loosen `audit_write`.
  - **Idempotency keys are per store.** Before the store page uses them, add who made the call (staff user, client account, or a store-page marker) to the key, so an anonymous caller can neither replay nor probe another's key.
  - **Some columns clients must never see sit on rows they can read:** `order_items.expected_date`, `orders.cancel_reason`, `released_by` and `tracking_token_hash`, and `payments.recorded_by`, `voided_by` and `void_reason`. They are readable under `client_read`, and under `tenant_isolation` by a store-page scope. Client-area and tracking repositories therefore name their columns and never load the admin models. Consider a `security_invoker` view with only the client columns. Customers' phones and emails are readable by a store-page scope too; M7 needs that to match a customer by phone, so the app layer is the only guard there.
  - **Low:** the payment grants would let a void be undone, which only the domain stops. And any scope of the store may delete a live idempotency key: limit deletes to expired keys when the worker takes them over.
- `order.deposit_confirmed` fires again when a voided deposit is paid again, so M5's `GeneratePurchases` must be idempotent per order item: a line that already exists for an item is never made twice.
- M7's `PlaceOrder` reuses the find-by-phone path of `Create`. On the public store page a phone proves nothing, so for a customer matched only by phone it must answer no customer facts (name, addresses) and must not use the stored addresses; the delivery address comes from the checkout form (security review of 4.1, D5). 4.10 writes this in `app/order/doc.go`.
