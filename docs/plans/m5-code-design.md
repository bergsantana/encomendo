# Milestone 5 — code design

Written in step 5.1 on Opus, against `main` at `4868e2b`. It is the code-level design every later step of `docs/plans/m5-purchasing-receiving.md` builds from: packages, types, signatures, SQL, endpoints, problem codes, web files and test names. ADR 0031 holds the decisions. The contract is in `docs/api/openapi.yaml` (tag `purchasing`, plus `listSupplierItems`). A step that finds this wrong stops and says so, then amends this file in its PR, rather than redesigning.

**The one rule that shapes everything:** purchasing depends on orders, and orders never know purchasing (`backend/CLAUDE.md`: no two contexts with ports to each other). "A comprar" is read from `app/order`; a `purchase_lines` row is written only when a line is sent or received at the counter.

```text
adapter/http ──► app/purchasing ──OrderItems──► adapter/inproc ──► app/order
                       │ ──Suppliers──► inproc ──► app/supplier
                       │ ──Store──────► inproc ──► app/tenancy (store name)
                       └ ──ReceiptPhotos──► adapter/storage (S3)
app/order ──► (nothing of purchasing)
```

## 1. Domain

### 1.1 `domain/shared` additions (step 5.2)

`id.go` gains `SupplierOrderID`, `PurchaseLineID` and `ReceiptID`, built like `PaymentID` (type, `Parse…`, `String`, `IsZero`).

### 1.2 `domain/purchasing` (steps 5.2, 5.3)

Files: `doc.go`, `errors.go`, `number.go`, `line.go`, `supplier_order.go`, `message.go`, `receipt.go`, `notice.go`, each with tests.

```go
// number.go
type Number int                       // per store, from 1
func (n Number) String() string       // "C-0001", "C-12345"

// line.go
type LineStatus string
const (
    LineSent      LineStatus = "sent"
    LineInTransit LineStatus = "in_transit"
    LineReceived  LineStatus = "received"
    LineDefective LineStatus = "defective"
    LineCancelled LineStatus = "cancelled"
)
func (s LineStatus) Open() bool       // sent or in_transit

type CancelReason string              // "undone" | "resolved"

// Choice is an option choice as the order item was sold. Options are store
// choices, never free text (ADR 0024), so they carry no client data.
type Choice struct{ Option shared.OptionID; ChoiceID shared.ChoiceID; Name, Label string }

// Item is what a line buys, copied from the order item when it is sent.
type Item struct {
    Order       shared.OrderID
    OrderItem   shared.OrderItemID   // zero once removed from the order
    CatalogItem shared.ItemID
    Name        string
    Quantity    int                  // 1..99
    Choices     []Choice
}

type Line struct {            // unexported fields; getters below
    id            shared.PurchaseLineID
    supplierOrder shared.SupplierOrderID // zero for a counter receipt
    supplier      shared.SupplierID
    item          Item
    status        LineStatus
    promised      shared.CivilDate       // the line's own date; zero = the supplier order's
    cost, paid    *shared.Money          // unit; owner only
    checkNote     string
    cancelReason  CancelReason
    receivedOn    shared.CivilDate
}
```

`Line` getters: `ID`, `SupplierOrder`, `Supplier`, `Item`, `Status`, `Promised`, `Cost`, `Paid`, `CheckNote`, `CancelReason`, `ReceivedOn`. `LineRecord` and `RestoreLine` follow the `order.Record` pattern.

```go
// supplier_order.go
type SupplierOrder struct {
    tenant      shared.TenantID
    id          shared.SupplierOrderID
    number      Number
    supplier    shared.SupplierID
    supplierName, supplierPhone string    // as sent
    sentAt      time.Time
    sentBy      shared.UserID
    promised    shared.CivilDate
    inTransitAt time.Time
    undoneAt    time.Time
    undoneBy    shared.UserID
    lines       []Line
    version     int
}

type SendSpec struct {
    Tenant   shared.TenantID
    ID       shared.SupplierOrderID
    Number   Number
    Supplier shared.SupplierID
    SupplierName, SupplierPhone string
    Lines    []LineSpec           // one per picked item, each with its id and cost
    By       shared.Principal
    Now      time.Time
}
type LineSpec struct{ ID shared.PurchaseLineID; Item Item } // no cost: the repository copies it in SQL (§4.1)

// Send makes a supplier order with every line sent. ErrEmptyPick for no
// lines, ErrItemPickedTwice for one order item twice, a ValidationError for
// a bad quantity or name.
func Send(s SendSpec) (*SupplierOrder, error)

func (o *SupplierOrder) State() State     // sent | in_transit | received | undone (contract SupplierOrderState)
func (o *SupplierOrder) CanUndo() bool    // no line received or defective, not undone
// Undo cancels every line ("undone") and returns their order items, to go
// back to to_buy. ErrSupplierOrderReceived once a line was received or
// found defective; ErrUndone when already undone.
func (o *SupplierOrder) Undo(by shared.Principal, now time.Time) ([]shared.OrderItemID, error)
// MarkInTransit moves every sent line to in_transit and returns their items.
// ErrNothingOpen when no line is sent.
func (o *SupplierOrder) MarkInTransit(now time.Time) ([]shared.OrderItemID, error)
// RecordPromisedDate sets the order's date and each override, for open lines
// only. ErrDateInPast for a day before today (field "date" or
// "lines[i].date"); ErrLineNotFound for a line not on it; ErrLineClosed for
// an override on a closed line. Returns each open line's item with its date.
func (o *SupplierOrder) RecordPromisedDate(date shared.CivilDate, overrides map[shared.PurchaseLineID]shared.CivilDate, today shared.CivilDate) ([]ItemDate, error)
// RecordPaidPrices sets the unit price paid on lines not cancelled.
// ErrLineNotFound, ErrLineClosed for a cancelled line. Returns each line's
// item with its price, for the order's margin.
func (o *SupplierOrder) RecordPaidPrices(prices map[shared.PurchaseLineID]shared.Money) ([]ItemPrice, error)
// Receive checks one open line. ErrLineClosed when not open; a
// ValidationError on "note" for a defect with no note or over 500
// characters. A defect keeps the note.
func (o *SupplierOrder) Receive(line shared.PurchaseLineID, check Check, note string, today shared.CivilDate) (Line, error)
// Resolve cancels an open line that is a notice ("resolved").
// ErrLineClosed when not open.
func (o *SupplierOrder) Resolve(line shared.PurchaseLineID) (Line, error)

type ItemDate  struct{ Order shared.OrderID; Item shared.OrderItemID; Date shared.CivilDate }
type ItemPrice struct{ Order shared.OrderID; Item shared.OrderItemID; Price shared.Money }
```

`Check` is `ok` or `defective`. purchasing keeps its own type, with values equal to `order.Check`, and the inproc adapter maps one to the other, so the domains don't import each other.

```go
// receipt.go
// CounterLine is a line for an item received while still in "A comprar"
// (bought at the counter): received or defective at once, with no supplier
// order.
type CounterSpec struct{ ID shared.PurchaseLineID; Supplier shared.SupplierID; Item Item; Check Check; Note string; Today shared.CivilDate }
func CounterLine(s CounterSpec) (Line, error)

type Receipt struct {
    tenant      shared.TenantID
    id          shared.ReceiptID
    by          shared.UserID
    at          time.Time
    entries     []ReceiptEntry      // 1..200, one line each, no line twice
    invoice     []Photo             // 0..4
}
type ReceiptEntry struct{ Line shared.PurchaseLineID; Check Check; Note string; Photos []Photo } // 0..2 photos
type Photo struct{ ID shared.PhotoID; Key string; ContentType string; Size int64 }

// NewReceipt validates the limits: ErrEmptyReceipt, ErrLineTwice,
// ErrPhotoLimit (field "invoicePhotoKeys" or "entries[i].photoKeys"; at
// most 20 photos in all, so one request cannot hold a transaction over
// hundreds of storage calls).
func NewReceipt(spec ReceiptSpec) (*Receipt, error)

// PhotoKey returns a receiving photo's object key: t/{tenant}/receipts/{photo}.
func PhotoKey(tenant shared.TenantID, photo shared.PhotoID) string
func ParsePhotoKey(tenant shared.TenantID, key string) (shared.PhotoID, error) // ErrPhotoKey for anything else
```

Photo content checks (size ≤ 10 MB, JPEG, PNG or WebP by sniffing) reuse `domain/catalog`'s sniffing function, which a domain package may import. If it is unexported, 5.2 moves it to `domain/shared` as `SniffPhoto` and catalog calls it from there.

```go
// notice.go
type NoticeReason string   // order_cancelled | item_removed | item_changed

// OrderSide is what the order context says of a line's item now.
type OrderSide struct {
    OrderCancelled bool
    ItemRemoved    bool       // the order has no such item (or the line's item is zero)
    Quantity       int
    Choices        []Choice
}
// NoticeOf reports why an open line is no longer needed, and false when it
// still is (or is not open).
func NoticeOf(l Line, now OrderSide) (NoticeReason, bool)
```

`item_changed` means the quantity differs, or the set of (option, choice) pairs differs.

```go
// message.go
type MessageFacts struct {
    Store, Supplier string
    Number          Number   // zero for "Falar com o fornecedor" before sending
    Items           []Item
}
// OrderMessage, TalkMessage and CancelMessage return the pt-BR texts of
// docs/product/design-document.md, "Supplier messages". OrderMessage sums
// items with the same catalog item and the same set of choices, keeping
// the order of first appearance.
//
// Product rule: they carry the store's and the supplier's names only, never
// the client's name, phone, address or order number
// (docs/product/design-document.md, System rules). Item holds no such field,
// so no caller can pass one.
func OrderMessage(f MessageFacts) string
func TalkMessage(f MessageFacts) string     // one item
func CancelMessage(f MessageFacts) string   // one item
```

**Errors** (`errors.go`): `ErrEmptyPick`, `ErrItemPickedTwice`, `ErrSupplierOrderReceived`, `ErrUndone`, `ErrNothingOpen`, `ErrLineNotFound`, `ErrLineClosed`, `ErrDateInPast`, `ErrEmptyReceipt`, `ErrLineTwice`, `ErrPhotoLimit`, `ErrPhotoKey`, `ErrSupplierOrderNotFound`. Each refusal that names a field is also a `*shared.ValidationError`, as order's `refuse` does.

**Tests (named):**

- `TestNumber_String`
- `TestSend_RefusesEmptyAndTwice`
- `TestUndo_RefusedOnceReceived`
- `TestRecordPromisedDate_OverridesAndPastDays`
- `TestReceive_DefectNeedsNote`
- `TestCounterLine`
- `TestNoticeOf_EachReason`
- `TestNewReceipt_Limits`
- `TestPhotoKey_RoundTrip`
- `TestOrderMessage_SumsSameItemAndChoices`
- `TestOrderMessage_NeverCarriesClientData` (builds items whose order customer has a name, phone and address in the test only, and asserts none of them nor any `P-` number is in the text)
- `FuzzOrderMessage_Names` (names and choices with any runes never produce a `P-0` pattern or a phone-looking run of digits that was not in an item name)

### 1.3 `domain/order` changes (step 5.4)

- `Item` gains `promised shared.CivilDate`. `timing(i)` sets `Promised: i.promised`. `ItemRecord` gains `Promised`.
- `Sourcing` gains `PaidCost *shared.Money` (unit; owner only).
- `HistoryEntry` gains `Ref string` (the pedido de compra number), and `HistoryKind` gains `HistoryPromised` ("promised_date").

```go
var ErrItemMovedByPurchasing = errors.New("order: bought items move through purchasing")

// bought reports whether an item's mode is bought: to_order, and stock
// until milestone 6 (ADR 0030, ADR 0031).
func bought(m catalog.SourcingMode) bool

// MoveItem, as today, but for a bought item every move except received →
// installed is ErrItemMovedByPurchasing, checked before CanMoveItem.

// PurchaseMove is a move purchasing makes: to sent_to_supplier, in_transit,
// received (with check) or to_buy (an undo, or a resolved changed notice).
type PurchaseMove struct {
    Item  shared.OrderItemID
    To    ItemStatus
    Check *Check
    Note  string   // a defect's note, kept in the history entry's Reason
    Ref   string   // "C-0012", or "" for a counter receipt
}
// MovePurchased applies m on behalf of by. It returns ErrOrderClosed,
// ErrItemNotFound, ErrInvalidItemMove for an in-house item or a move the
// bought lifecycle lacks (CanMoveItem; to_buy is allowed from
// sent_to_supplier and in_transit as an undo), and lands a defect on to_buy.
// Landing on to_buy clears the item's promised date and redates it from
// today; received sets arrivedOn.
func (o *Order) MovePurchased(m PurchaseMove, today shared.CivilDate, now time.Time, by shared.Principal) error

// RecordPromisedDate sets the supplier's promised date of a bought item
// that is sent_to_supplier or in_transit, and redates it: expected =
// promised + its options' extra days. The promise never moves (ADR 0030).
// ErrOrderClosed, ErrItemNotFound, ErrInvalidItemMove for any other item,
// ErrDateInPast before today.
func (o *Order) RecordPromisedDate(item shared.OrderItemID, date shared.CivilDate, ref string, today shared.CivilDate, now time.Time, by shared.Principal) error

// RecordPaidCost sets the unit price paid for a bought item. It records no
// history: history is read by every staff role, costs by the owner only.
// ErrItemNotFound; an amount in another currency is ErrInvalid.
func (o *Order) RecordPaidCost(item shared.OrderItemID, cost shared.Money) error

// Redate recomputes the forecast of every item not arrived (the unexported
// redate). The promise never moves.
func (o *Order) Redate(today shared.CivilDate)

// Purchasable reports whether the order's bought items in to_buy belong in
// "A comprar": not closed, and released, or past draft when the store buys
// before the deposit (ADR 0031).
func (o *Order) Purchasable(beforeDeposit bool) bool
```

The existing `TestAdvance_<Preset>_EveryMove_WithGuardHeldAndBroken` tables move their bought items with `MovePurchased` instead of `MoveItem`.

**Tests (named):**

- `TestMoveItem_BoughtOnlyInstalls`
- `TestMovePurchased_Lifecycle`
- `TestMovePurchased_DefectLandsOnToBuyAndClearsPromise`
- `TestRecordPromisedDate_LateRaisesAtRiskKeepsPromise`
- `TestRecordPaidCost`
- `TestPurchasable_EveryPhase`
- `TestRedate_KeepsArrived`

## 2. Migration `0016_purchasing.sql` (step 5.5)

```sql
ALTER TABLE order_item_sourcing
    ADD COLUMN promised_date   date,
    ADD COLUMN paid_cost_cents bigint CHECK (paid_cost_cents BETWEEN 0 AND 100000000000);
-- The promised date is the supplier's: staff-only, so it lives in sourcing,
-- never in order_items, which has a client read policy (0013).
-- The C-number rides in order_status_history.detail (jsonb), as every
-- other HistoryEntry field; no column.
-- 0013's CHECK on order_status_history.kind lists the M4 kinds: drop and
-- re-add it with 'promised_date' (Down restores the old list).
-- 0013 grants UPDATE on order_item_sourcing by column: add the new ones.
GRANT UPDATE (promised_date, paid_cost_cents) ON order_item_sourcing TO encomendo_app;
-- An item and its order, tied for the line's foreign key below.
ALTER TABLE order_items ADD CONSTRAINT order_items_tenant_order_id_key UNIQUE (tenant_id, order_id, id);

CREATE TABLE purchase_counters (   -- as order_counters
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    last_number int NOT NULL DEFAULT 0 CHECK (last_number >= 0),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id));

CREATE TABLE supplier_orders (
    tenant_id      uuid NOT NULL,
    id             uuid NOT NULL,
    number         int  NOT NULL CHECK (number >= 1),
    supplier_id    uuid NOT NULL,
    supplier_name  text NOT NULL,
    supplier_phone text NOT NULL,
    sent_at        timestamptz NOT NULL,
    sent_by        uuid NOT NULL,   -- no FK: records outlive the account, as payments
    promised_date  date,
    in_transit_at  timestamptz,
    undone_at      timestamptz,
    undone_by      uuid,
    message_text   text NOT NULL,   -- as sent, no client data (ADR 0031)
    message_url    text NOT NULL,
    version        int  NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id), UNIQUE (tenant_id, id), UNIQUE (tenant_id, number),
    FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, supplier_id) REFERENCES suppliers (tenant_id, id),
    CHECK ((undone_at IS NULL) = (undone_by IS NULL)));
CREATE INDEX supplier_orders_tenant_sent_idx ON supplier_orders (tenant_id, sent_at DESC, id DESC);

CREATE TABLE purchase_lines (
    tenant_id         uuid NOT NULL,
    id                uuid NOT NULL,
    supplier_order_id uuid,                 -- null for a counter receipt
    supplier_id       uuid NOT NULL,
    order_id          uuid NOT NULL,        -- orders are never deleted
    order_item_id     uuid,                 -- null once removed from the order (M6's restock gets its own column)
    catalog_item_id   uuid NOT NULL,
    name              text NOT NULL,
    quantity          int  NOT NULL CHECK (quantity BETWEEN 1 AND 99),
    choices           jsonb NOT NULL DEFAULT '[]',  -- [{option, choice, name, label}]
    status            text NOT NULL CHECK (status IN ('sent','in_transit','received','defective','cancelled')),
    promised_date     date,
    check_note        text NOT NULL DEFAULT '' CHECK (length(check_note) <= 500),
    cancel_reason     text CHECK (cancel_reason IN ('undone','resolved')),
    received_on       date,
    position          int  NOT NULL CHECK (position >= 0),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id), UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE,  -- deleting a store (audit_test.go)
    FOREIGN KEY (tenant_id, supplier_order_id) REFERENCES supplier_orders (tenant_id, id),
    FOREIGN KEY (tenant_id, supplier_id) REFERENCES suppliers (tenant_id, id),
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id),
    FOREIGN KEY (tenant_id, order_id, order_item_id) REFERENCES order_items (tenant_id, order_id, id) ON DELETE SET NULL (order_item_id),
    CHECK ((status = 'cancelled') = (cancel_reason IS NOT NULL)),
    CHECK (supplier_order_id IS NOT NULL OR status IN ('received','defective')),
    CHECK ((status IN ('received','defective')) = (received_on IS NOT NULL)),
    CHECK (status <> 'defective' OR check_note <> ''));
CREATE UNIQUE INDEX purchase_lines_open_item_idx ON purchase_lines (tenant_id, order_item_id)
    WHERE status IN ('sent','in_transit');
CREATE INDEX purchase_lines_tenant_order_idx ON purchase_lines (tenant_id, order_id);
CREATE INDEX purchase_lines_tenant_supplier_order_idx ON purchase_lines (tenant_id, supplier_order_id, position);

-- Costs apart, not audited: audit_log is read by every staff role, costs by the owner (0013, ADR 0025).
CREATE TABLE purchase_line_costs (
    tenant_id       uuid NOT NULL,
    line_id         uuid NOT NULL,
    currency        text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    cost_cents      bigint CHECK (cost_cents BETWEEN 0 AND 100000000000),
    paid_cost_cents bigint CHECK (paid_cost_cents BETWEEN 0 AND 100000000000),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, line_id),
    FOREIGN KEY (tenant_id, line_id) REFERENCES purchase_lines (tenant_id, id) ON DELETE CASCADE);

CREATE TABLE receipts (
    tenant_id   uuid NOT NULL,
    id          uuid NOT NULL,
    received_by uuid NOT NULL,
    received_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id), UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE);

CREATE TABLE receipt_lines (
    tenant_id  uuid NOT NULL,
    receipt_id uuid NOT NULL,
    line_id    uuid NOT NULL,
    check_result text NOT NULL CHECK (check_result IN ('ok','defective')),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, receipt_id, line_id),
    UNIQUE (tenant_id, line_id),     -- a line is received once
    FOREIGN KEY (tenant_id, receipt_id) REFERENCES receipts (tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, line_id) REFERENCES purchase_lines (tenant_id, id));

CREATE TABLE receipt_photos (
    tenant_id    uuid NOT NULL,
    id           uuid NOT NULL,
    receipt_id   uuid NOT NULL,
    line_id      uuid,               -- null for an invoice photo
    object_key   text NOT NULL,
    content_type text NOT NULL CHECK (content_type IN ('image/jpeg','image/png','image/webp')),
    size_bytes   bigint NOT NULL CHECK (size_bytes BETWEEN 1 AND 10485760),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id), UNIQUE (tenant_id, id), UNIQUE (object_key),
    FOREIGN KEY (tenant_id, receipt_id) REFERENCES receipts (tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, receipt_id, line_id) REFERENCES receipt_lines (tenant_id, receipt_id, line_id),
    CHECK (object_key = 't/' || tenant_id || '/receipts/' || id));
CREATE INDEX receipt_photos_tenant_receipt_idx ON receipt_photos (tenant_id, receipt_id);
```

- Every table: `ENABLE` and `FORCE ROW LEVEL SECURITY`, and the **`staff_only`** policy of 0013 (a tenant **and** a staff user). There is no client policy anywhere. `purchase_counters` gets the same policy.
- Grants to `encomendo_app`: `SELECT, INSERT` on every table, `DELETE` on none (nothing is deleted: lines are cancelled, and a line keeps its row when its order item goes), and `UPDATE` only on the columns that change, so the database keeps a line's order item, supplier and snapshot fixed and receipts append-only:

  ```sql
  GRANT UPDATE (status, promised_date, check_note, cancel_reason, received_on, updated_at) ON purchase_lines TO encomendo_app;
  GRANT UPDATE (promised_date, in_transit_at, undone_at, undone_by, version, updated_at) ON supplier_orders TO encomendo_app;
  GRANT UPDATE (paid_cost_cents, updated_at) ON purchase_line_costs TO encomendo_app;
  GRANT UPDATE (last_number, updated_at) ON purchase_counters TO encomendo_app;
  -- receipts, receipt_lines, receipt_photos: SELECT, INSERT only.
  ```

  `order_item_id` needs no grant: the set-null action runs as the table owner.
- **No audit triggers** (as built in 5.5): `audit_log` is read by every staff role, viewers included, and these rows name suppliers and their WhatsApp. No staff-only table is audited (0009, 0013; the `/new-migration` skill; `TestAuditTriggers_OnlyOnTheListedTables`).
- **The `ON DELETE SET NULL (order_item_id)` trap:** Postgres runs a referential action as the table owner and **ignores RLS**, so `staff_only` does not guard it (the tenancy review saw a store page scope's `DELETE FROM order_items` null a line it cannot read). The guard is `hasStaff` in the order repository, which refuses an update without a staff scope before any delete. The audit trigger runs after the statement as `encomendo_app` and records `app.user_id`. 5.5 tests:
  - `TestPurchaseLine_KeepsRowWhenOrderItemRemoved`, where a staff-scope delete keeps the line with a null item;
  - the order repository's existing case "those scopes write nothing through the repository" (`test/integration/postgres/order_repo_test.go`), which refuses a store page's `Update` before any write;
  - and the M7 allow-list gains the trap "a store page scope must never delete `order_items`".
- Down: drop the tables in reverse, then the three columns.

**RLS suite (5.5):**

- the six cases for each new table: select, insert with another tenant, update, delete, no tenant, and a store page scope;
- `TestClientRead_PurchasingTablesHaveNoClientPolicy`;
- `TestPurchaseLines_OneOpenLinePerItem`;
- `TestPurchaseLines_CrossTenantForeignKeysRefused`;
- `TestPurchaseLine_KeepsRowWhenOrderItemRemoved`;
- `TestPurchaseLine_ItemMustBelongToItsOrder`;
- `TestTenantDelete_CascadesPurchasing` (a store with a sent line, a counter line, a receipt and a photo row is deleted cleanly, as `TestAuditTrigger_SkipsADeletedStoresCascade`).

**M7 allow-list:** `order_item_sourcing.promised_date` and `paid_cost_cents` join the list of traps (they are staff-only anyway). 5.5 adds that line to `docs/plans/m4-customers-orders-deadlines.md`'s list.

## 3. Application layer

### 3.1 `app/order` purchasing entry points (step 5.6)

`app/order/purchasing.go`. They are on a second driving port, so `UseCases` stays the HTTP adapter's:

```go
// ForPurchasing is what purchasing calls through adapter/inproc
// (ADR 0031). Every method checks its own permission and runs in one
// WithinTx, which joins the caller's transaction through a savepoint.
type ForPurchasing interface {
    ToBuy(ctx context.Context) (ToBuyView, error)                                     // purchase:write or receiving:write
    PurchaseOrders(ctx context.Context, ids []shared.OrderID) ([]PurchaseOrderView, error) // order:read
    MovePurchased(ctx context.Context, moves []PurchaseMoveInput) error               // see below
    RecordPromisedDates(ctx context.Context, dates []PromisedDateInput) error          // purchase:write
    RecordPaidCosts(ctx context.Context, costs []PaidCostInput) error                  // purchase:write + SeesCosts
}

type ToBuyView struct { Items []BoughtItemView; Truncated bool }
type BoughtItemView struct {
    Order        shared.OrderID
    Number       order.Number
    Item         shared.OrderItemID
    CatalogItem  shared.ItemID
    Name         string
    Quantity     int
    Options      []order.ChosenOption
    Supplier     shared.SupplierID
    SupplierName string
    ArrivalLimit shared.CivilDate
    Cost         *shared.Money // nil unless the caller sees costs; never stored from here (§4.1)
}
type PurchaseOrderView struct {
    Order        shared.OrderID
    Number       order.Number
    Cancelled    bool
    ArrivalLimit shared.CivilDate
    Items        []PurchaseItemView // every item, bought or not
}
type PurchaseItemView struct {
    Item         shared.OrderItemID
    Bought       bool
    Status       order.ItemStatus
    Quantity     int
    Options      []order.ChosenOption
    Expected     shared.CivilDate
    Promised     shared.CivilDate
    Alerts       []schedule.Alert
    Supplier     shared.SupplierID
    SupplierName string
}
type PurchaseMoveInput struct {
    Order shared.OrderID
    Move  order.PurchaseMove
}
type PromisedDateInput struct{ Order shared.OrderID; Item shared.OrderItemID; Date shared.CivilDate; Ref string }
type PaidCostInput     struct{ Order shared.OrderID; Item shared.OrderItemID; Cost shared.Money }
```

- **`ToBuy`** reuses `s.active` (500 cap, `Truncated`) and `restoreAll`. It keeps `o.Purchasable(facts.PurchaseBeforeDeposit)` orders and their bought items in `to_buy`. `StoreFacts` gains `PurchaseBeforeDeposit`, filled by `adapter/inproc/order_ports.go` from tenant settings.
- **`MovePurchased`'s permission per move:** `to = received` needs `receiving:write`; every other move needs `purchase:write`. It groups moves by order and loads each order once. It saves with the loaded version (the caller holds none), so a concurrent order write is `shared.ErrVersionConflict` and the purchasing call fails whole. It publishes `order.ItemMoved` for each move.
- **`RecordPaidCosts`** refuses a caller who doesn't see costs with `shared.ErrForbidden`.

**Tests (named):**

- `TestToBuy_AfterDepositNotBefore`
- `TestToBuy_BeforeDepositSettingPastDraft`
- `TestToBuy_DepositPaidAgainShowsOnce`
- `TestToBuy_LeavesOutCancelledRemovedInHouse`
- `TestToBuy_BackupSwitchShowsBackupSupplier`
- `TestToBuy_CostOnlyForOwner`
- `TestMovePurchased_PermissionPerMove` (keyed on the requested `To`: a staff "Com defeito" receipt, which lands on `to_buy`, stays `receiving:write`)
- `TestMovePurchased_VersionConflictFailsTheCall`
- `TestMovePurchased_TwoMovesOnOneOrder`
- `TestMovePurchased_JoinsOuterTxAndRollsBack`
- `TestRecordPaidCosts_OwnerOnly`
- `TestPurchaseOrders_RemovedItemAbsent`

### 3.2 `app/purchasing` (steps 5.6 ports and fake, 5.7 service)

Files: `doc.go`, `ports.go`, `inputs.go`, `views.go`, `service.go`, `send.go`, `receive.go`, `queries.go`, `today.go`, `errors.go`, `fake/`.

```go
// ports.go
type UseCases interface {
    ToBuy(ctx context.Context) (ToBuyView, error)                                        // purchase:write
    Preview(ctx context.Context, in PickInput) (string, error)                           // purchase:write
    Send(ctx context.Context, in SendInput) (SentView, error)                            // purchase:write; Idempotency-Key
    SupplierOrders(ctx context.Context, open bool, p shared.PageRequest) (shared.Page[SupplierOrderSummary], error)
    SupplierOrder(ctx context.Context, id shared.SupplierOrderID) (SupplierOrderView, error)
    RecordPromisedDate(ctx context.Context, in PromisedDateInput) (SupplierOrderView, error)
    MarkInTransit(ctx context.Context, in VersionInput) (SupplierOrderView, error)
    Undo(ctx context.Context, in VersionInput) (SupplierOrderView, error)
    RecordPaidPrices(ctx context.Context, in PaidPricesInput) (SupplierOrderView, error) // owner
    ToReceive(ctx context.Context) (ToReceiveView, error)                                // receiving:write
    RequestPhotoUpload(ctx context.Context, contentType string) (PhotoUploadView, error) // receiving:write
    Receive(ctx context.Context, in ReceiptInput) (ReceiptView, error)                   // receiving:write; Idempotency-Key
    Notices(ctx context.Context) ([]NoticeView, error)                                   // purchase:write
    ResolveNotice(ctx context.Context, line shared.PurchaseLineID) error                 // purchase:write
    OrderLines(ctx context.Context, id shared.OrderID) ([]OrderLineView, error)          // order:read; chat link only for purchase:write
    Today(ctx context.Context) (TodayView, error)                                        // purchase:write
}

// OrderItems is purchasing's view of orders (adapter/inproc/purchasing_orders.go
// over app/order's ForPurchasing). Types are purchasing's own.
type OrderItems interface {
    ToBuy(ctx context.Context) (ToBuyItems, error)
    Orders(ctx context.Context, ids []shared.OrderID) ([]OrderFacts, error)
    Move(ctx context.Context, moves []ItemMove) error
    RecordPromisedDates(ctx context.Context, dates []ItemPromise) error
    RecordPaidCosts(ctx context.Context, costs []ItemCost) error
}

type Repository interface {
    NextNumber(ctx context.Context, tenant shared.TenantID) (purchasing.Number, error)
    PeekNumber(ctx context.Context, tenant shared.TenantID) (purchasing.Number, error) // for Preview; takes nothing
    InsertSupplierOrder(ctx context.Context, o *purchasing.SupplierOrder, text, url string) error
    UpdateSupplierOrder(ctx context.Context, o *purchasing.SupplierOrder) error  // version check
    SupplierOrder(ctx context.Context, tenant shared.TenantID, id shared.SupplierOrderID) (purchasing.SupplierOrderRecord, error)
    SupplierOrderOfLine(ctx context.Context, tenant shared.TenantID, line shared.PurchaseLineID) (purchasing.SupplierOrderRecord, error)
    SupplierOrders(ctx context.Context, tenant shared.TenantID, open bool, p shared.PageRequest) (shared.Page[purchasing.SupplierOrderRecord], error)
    OpenLines(ctx context.Context, tenant shared.TenantID) ([]purchasing.LineRecord, error) // with their supplier order's number and name
    LinesOfOrder(ctx context.Context, tenant shared.TenantID, id shared.OrderID) ([]purchasing.LineRecord, error)
    DefectiveItems(ctx context.Context, tenant shared.TenantID, items []shared.OrderItemID) (map[shared.OrderItemID]bool, error)
    InsertCounterLine(ctx context.Context, l purchasing.Line) error
    InsertReceipt(ctx context.Context, r *purchasing.Receipt) error
    StaffNames(ctx context.Context, ids []shared.UserID) (map[shared.UserID]string, error)
}

type Suppliers interface {   // inproc over app/supplier
    Contact(ctx context.Context, id shared.SupplierID) (SupplierContact, error) // name, WhatsApp
}
type Store interface { Name(ctx context.Context) (string, error); Today(ctx context.Context) (shared.CivilDate, error) }
type SupplierLinks interface { Link(phone, text string) (string, error) } // adapter/notify, as order's WhatsApp port
type ReceiptPhotos interface { // adapter/storage.ReceiptStore over S3
    PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (SignedUpload, error)
    Stat(ctx context.Context, key string) (ObjectInfo, error)
    Head(ctx context.Context, key, etag string, n int) ([]byte, error)
    Promote(ctx context.Context, key, etag, contentType string) error
    DeleteUpload(ctx context.Context, key string) error
    ReportOrphan(ctx context.Context, key string)
}
```

`app/purchasing` declares its own `SignedUpload`, `ObjectInfo`, `ErrObjectMissing` and `ErrObjectChanged`, because it may not import `app/catalog`. `adapter/storage/receipts.go` wraps `*S3` and maps them. It also takes `appshared.Idempotency`, `TxManager`, `Clock` and `IDGenerator`, and no `EventPublisher`: purchasing publishes no events in M5.

**Use case flows:**

- **`Send`:** `authz.Require(purchase:write)`, then the Idempotency-Key, then `WithinTx`:
  - replay check (the key's resource is a supplier order id, else `ErrIdempotencyKeyReused`);
  - `OrderItems.ToBuy`; every picked id must be there for `in.Supplier`, else `ErrItemNotToBuy`;
  - `Suppliers.Contact`, `Store.Name`, `NextNumber`;
  - `purchasing.Send`, then `OrderMessage`, then `SupplierLinks.Link`;
  - `InsertSupplierOrder`, which copies each line's order-time cost from `order_item_sourcing` in SQL, in the same transaction, whoever sends (an admin never sees costs, and the owner must still find them; §4.1);
  - `OrderItems.Move` (to `sent_to_supplier`, `Ref` = number);
  - `Idempotency.Remember`.
  The unique open-line index turns a race into `shared.ErrConflict`, which `Send` maps to `ErrItemNotToBuy`.
- **`Undo`:** `so.Undo`, then `Update`, then `Move` each item to `to_buy`. A closed order (cancelled) is skipped: its items need no move.
- **`RecordPromisedDate`:** `so.RecordPromisedDate`, then `Update`, then `OrderItems.RecordPromisedDates` for items still on an open order.
- **`MarkInTransit`:** `so.MarkInTransit`, then `Update`, then `Move` to `in_transit`. Items of cancelled orders or removed items are skipped.
- **`RecordPaidPrices`:** owner only (`identity.SeesCosts`, else `shared.ErrForbidden`). `so.RecordPaidPrices`, then `Update`, then `OrderItems.RecordPaidCosts` for items still present.
- **`Receive`:** `receiving:write`, then the Idempotency-Key.
  1. **Before the transaction**, validate the photo keys: `ParsePhotoKey`, `Stat`, and sniff with `Head`, at most 10 MB and 20 photos in all. Storage calls never hold a transaction.
  2. `WithinTx`:
     - the replay check first, as `Send` does: a key whose resource is a receipt answers it again, whatever state the lines are now in;
     - for each `lineId` entry: `SupplierOrderOfLine`, `so.Receive`, `Update` (each supplier order loaded once);
     - for each `orderItemId` entry: it must be in `OrderItems.ToBuy`, else `ErrItemNotToBuy`; then `purchasing.CounterLine` and `InsertCounterLine` (the cost is copied in SQL, as in `Send`);
     - `NewReceipt`, then `InsertReceipt`;
     - `OrderItems.Move`: `received` with `ok`, or with `defective` and its note, which lands on `to_buy`. **Lines whose order was cancelled or whose item was removed are received but not moved**, as `Undo` and `MarkInTransit` skip them (ADR 0031: such an item "is still received");
     - `Promote` each photo, last, as ADR 0028 does; a failed promote rolls back;
     - `Idempotency.Remember`.
  3. After the transaction it reads `OrderItems.Orders` for `completedOrders` (every item arrived).
- **`Notices`:** `OpenLines`, then `OrderItems.Orders` for their orders, then `NoticeOf`. Each notice gets `CancelMessage` and a link.
- **`ResolveNotice`:** checks it is a notice now (`ErrLineClosed` otherwise), then `so.Resolve` and `Update`. For `item_changed` it also moves the item to `to_buy`, so it is bought again as it is now.
- **`ToBuy` (the purchasing view):** `OrderItems.ToBuy`, grouped by supplier (by name, then id). `DefectiveItems` marks "Reposição". `Suppliers.Contact` gives each phone.
- **`ToReceive`:** `OpenLines` grouped by supplier order, oldest send first; then `OrderItems.ToBuy` as the counter list.
- **`OrderLines`:** `order:read`. `OrderItems.Orders([id])`, plus `LinesOfOrder` (the open line, or else the latest). With `purchase:write`, each bought item gets a `TalkMessage` link (with the number once sent).
- **`Today`:** `purchase:write`, else `shared.ErrForbidden`:
  - `toSend` = the items in `ToBuy`;
  - needs: notices, then open supplier orders with no promised date sent more than 2 days ago (days in the store's zone), then items to send per supplier.

**App errors** (`errors.go`): `ErrItemNotToBuy`, `ErrIdempotencyKeyRequired`, `ErrReceiptNotFound`; the rest are the domain's.

**As built in 5.7** (the ports above, amended; `ports.go` is the source):
- `Store` also has `Currency(ctx)`, because prices paid arrive as bare cents, and `Location(ctx)`, so `Today` can count days in the store's zone.
- `Repository`:
  - `InsertCounterLine(ctx, tenant, l)` takes the tenant;
  - `Message(ctx, tenant, id)` answers a `Send` replay with the stored message;
  - `Receipt(ctx, tenant, id)` answers a `Receive` replay after the lines closed;
  - `OpenLines` and `LinesOfOrder` return `[]StoredLine`, the line plus its pedido de compra's number, name, phone, send time and promised date;
  - records are `purchasing.Record`.
- `Receive` runs the replay check in a small transaction before the photo checks, and again inside the main one.
- `Remember` runs before `Promote`, which is last; a promote left behind by a failed transaction is reported with `ReportOrphan`.
- A dry `NewReceipt` with placeholder photos refuses an over-limit request before any storage call.
- Bad uploads are `catalog.ErrPhotoNotUploaded` and `catalog.ErrPhotoInvalid`, so HTTP reuses `photo_not_uploaded` and `photo_invalid`.
- `Today` leaves out a pedido de compra's "sem prazo" need when every open line on it is a notice.

**Tests (named):**

- `TestService_EveryUseCaseChecksItsPermission` (a table over the 16)
- `TestSend_WritesLinesMovesItemsRecordsLink`
- `TestSend_ItemSentByAnotherIsNotToBuy`
- `TestSend_ReplayAnswersSameSupplierOrder`
- `TestSend_AdminSendKeepsOwnerCost`
- `TestSend_MessageHasNoClientData` (the send, notice and Falar texts, and the stored `message_text`, against every customer field of the orders behind them)
- `TestUndo_RefusedOnceReceived`
- `TestRecordPromisedDate_RedatesAndRaisesAtRisk`
- `TestRecordPaidPrices_OwnerOnly`
- `TestViews_NoCostForNonOwner`
- `TestReceive_DefectBackToBuyAsReposicao`
- `TestReceive_CounterLine`
- `TestReceive_PhotoChecks`
- `TestReceive_ReplayAnswersSameReceipt` (after the lines closed)
- `TestReceive_LineOfCancelledOrderIsReceivedNotMoved`
- `TestReceive_PhotoChecksRunBeforeTheTransaction`
- `TestReceive_CompletedOrders`
- `TestNotices_EachReason`
- `TestResolveNotice_ChangedItemBackToBuy`
- `TestOrderLines_ChatLinkOnlyForPurchaseWrite`
- `TestToday_NothingForStaff`
- `TestToday_NoPromisedDateAfterTwoDays`

### 3.3 `app/order` and `app/schedule` changes (step 5.8)

- `MoveItem` returns `order.ErrItemMovedByPurchasing` from the domain unchanged.
- `Cancel`: the hand-off comment is replaced by "Purchasing reads cancelled orders and flags their sent lines (ADR 0031)."
- `schedule.Recalculate`: load, `o.Redate(today)`, save (`Update`); with no change it saves nothing.
- `finance.go`: the margin's unit cost is `Sourcing.PaidCost` when set, else `Sourcing.Cost`.
- `Change` keeps a kept item's `promised` date (`keepState`) and its `Sourcing.PaidCost` (`lines.go` passes the old sourcing): `TestChange_KeepsPromisedDateAndPaidCost`.
- The order repository reads and writes `order_item_sourcing.promised_date` and `paid_cost_cents`, and `order_status_history.ref` (5.9).

**Tests (named):**

- `TestMoveItem_BoughtRefused`
- `TestRecalculate_SavesRedate`
- `TestFinance_MarginUsesPaidCost`

### 3.4 `app/catalog` addition (step 5.10)

`SupplierItems(ctx, supplier shared.SupplierID) ([]SupplierItemUseView, error)`: `catalog:read`, archived items left out, cost only with `SeesCosts`. It serves `listSupplierItems`.

**Test:** `TestSupplierItems_CostOwnerOnly`.

## 4. Adapters

### 4.1 `adapter/postgres` (step 5.9)

- `purchasing_repo.go`, `models/purchasing.go`.
- `NextNumber` as order's counter: `INSERT … ON CONFLICT DO UPDATE … RETURNING`. `PeekNumber` reads `last_number + 1` without writing.
- `OpenLines`: a `Raw` join of `purchase_lines` (status open) with `supplier_orders` (number, supplier name, `sent_at`), ordered by `sent_at`, `position`.
- Costs are read from and written to `purchase_line_costs` in the same transaction.
- 23505 maps by constraint: `purchase_lines_open_item_idx` to `shared.ErrConflict` (the app maps it to `ErrItemNotToBuy`); `receipt_photos_object_key_key` to the photo refusal `photo_not_uploaded`; `receipt_lines` (a line received twice) to `purchasing.ErrLineClosed`.
- **The repository refuses a scope without a staff user** before any write, as the order repository does (`hasStaff`), since referential actions bypass RLS.
- **Costs are copied, not passed:** `InsertSupplierOrder` and `InsertCounterLine` write `purchase_line_costs` with `INSERT … SELECT cost_cents FROM order_item_sourcing WHERE tenant_id = ? AND order_item_id = ?`, so a line sent by an admin keeps the owner's cost.
- Order repository changes: `ItemRecord.Promised`, `Sourcing.PaidCost` and `HistoryEntry.Ref`.
- **`pruneParts` must not delete and re-insert a kept item** (security review of 5.5). An item whose line id changed is deleted and inserted again under the same id. With 0016, that delete nulls the item's open purchase line, so Compras flags a false "Avisar o fornecedor" and the item can be sent twice. It also cascades `order_item_sourcing`, which loses `promised_date` and `paid_cost_cents`. 5.9 first confirms whether `app/order` ever keeps an item id under a new line id. If it does, either the domain refuses that for an item past `to_buy`, or the repository moves the item in place, which needs `UPDATE (line_id)` on `order_items` in a new migration. Test: `TestOrders_ChangeKeepsTheItemsPurchaseLine` (integration).

**Contract suite:** `test/contract/purchasing_repository.go`, run against `fake` and Postgres. It covers:

- the number never repeats under two concurrent sends;
- `PeekNumber` takes nothing;
- one open line per item;
- a version conflict;
- `OpenLines` order;
- `DefectiveItems`;
- `LinesOfOrder` after the item was removed.

Plus `test/contract/order_items.go` for `OrderItems`, run against the fake and the inproc adapter over `app/order` with its fakes.

### 4.2 `adapter/storage/receipts.go` (step 5.9)

`ReceiptStore{s *S3}` implements `apppurchasing.ReceiptPhotos`, mapping types and errors. Keys go to the same bucket, staged under `uploads/`, as ADR 0028 says.

**Test:** the Silo contract case `TestReceiptStore_StageAndPromote` (integration).

### 4.3 `adapter/notify` (step 5.10)

The existing `whatsapp_link.go` `Link(phone, text)` already builds `wa.me` links. `SupplierLinks` is the same adapter, through a second compile-time assertion.

### 4.4 `adapter/inproc` (steps 5.6, 5.10)

- `purchasing_orders.go`: `OrderItems` over `apporder.ForPurchasing`, mapping `purchasing.Check` ↔ `order.Check` and `Choice` ↔ `ChosenOption`.
- `purchasing_suppliers.go`: `Suppliers` over `appsupplier`.
- `purchasing_store.go`: `Store` over tenancy settings and the clock.

**Tests:** each has a compile-time assertion and a test against fakes.

### 4.5 Wiring (step 5.10)

In `main.go`, construct `apppurchasing.NewService(Deps{…})` after `apporder` and pass it to the router. Nothing takes the purchasing service but the router.

## 5. HTTP (steps 5.1, 5.10)

- `handler/purchasing.go` and `dto/purchasing.go`, one handler per operation of the `purchasing` tag. `listSupplierItems` goes in `handler/supplier.go`.
- Routes under `/api/v1/admin/purchasing` in `router.go`. The literal `preview` comes before `{supplierOrderId}`, as `/orders/quote` does.
- **Problem codes** (`respond/problem.go`), all `409`:
  - `item_moved_by_purchasing` (`order.ErrItemMovedByPurchasing`);
  - `item_not_to_buy` (`apppurchasing.ErrItemNotToBuy`);
  - `supplier_order_received` (`purchasing.ErrSupplierOrderReceived`);
  - `line_closed` (`purchasing.ErrLineClosed`, `ErrUndone`, `ErrNothingOpen`).
  Domain ValidationErrors are `400 invalid` with `errors`. `ErrSupplierOrderNotFound` and `ErrLineNotFound` (by path) are `404`. Photo refusals reuse `photo_invalid`, `photo_not_uploaded` and `photo_limit`.
- **Visibility tests:**
  - `TestPurchasingDTOs_CostOnlyForOwner`
  - `TestSupplierLinks_HaveNoClientData` (through HTTP with a fake app whose orders have customers)
  - `TestPurchasingDTOs_NotInClientTypes` (reflection: no purchasing DTO is reachable from `TrackingOrder`)
- **Rate limit:** `requestReceiptPhotoUpload` takes the catalog upload limit (ADR 0021).
- **API e2e** `test/e2e/purchasing_test.go`: deposit → "A comprar" → send → promised date → receive one item defective → it shows as "Reposição" → send again → receive ok → the order may move into assembling.

## 6. Web (steps 5.12–5.16)

### 6.1 `packages/api`

- `purchasing.ts`:
  - `toBuy`, `previewSupplierOrder`, `sendSupplierOrder` (key per action);
  - `supplierOrders`, `supplierOrder`, `recordPromisedDate`, `markInTransit`, `undoSupplierOrder`, `recordPaidPrices`;
  - `toReceive`, `notices`, `resolveNotice`, `orderPurchases`, `purchasingToday`.
- `receipts.ts`: `uploadReceiptPhoto(file)` (request, then upload, as `uploadPhoto` does) and `recordReceipt(body, key)`. The key is made once per "Salvar recebimento" and reused by "Tentar de novo".
- `suppliers.ts` gains `supplierItems(id)`.
- MSW handlers in the admin's test setup from the generated types.

### 6.2 Admin

Layers as `web/CLAUDE.md` says: `views/*.vue` → `store.ts` (Pinia) → `packages/api`.

- `features/compras/`:
  - `views/`: `ComprasView.vue` (tabs; `?tab=` keeps the tab on reload), `SupplierOrderView.vue` (route `/compras/pedidos/:id`), `ReceberView.vue` (route `/compras/receber`, also the staff menu entry).
  - `components/`:
    - `AComprarTab.vue` with `SupplierCard.vue`;
    - `EnviadasTab.vue`;
    - `NoticesPanel.vue`;
    - `FornecedoresTab.vue` with `SupplierDrawer.vue` (it reuses `catalogo/components/SupplierFields.vue`, moved to `packages/ui` or imported as is, never duplicated);
    - `PromisedDateDialog.vue`, `PaidPricesDialog.vue`, `UndoDialog.vue`;
    - `ReceiptLine.vue`, `PhotoPicker.vue`.
  - `store.ts` (`useComprasStore`, the feature's one store, `web/CLAUDE.md`):
    - to-buy, picks per supplier, supplier orders and notices;
    - the receipt being made: its checks, notes and photo states;
    - the `Idempotency-Key` made once per "Salvar recebimento" and kept for "Tentar de novo";
    - the result (`completedOrders`, `backToBuy`).
- Routes: `/compras` with `meta.permission: 'purchase:write'`; `/compras/receber` with `meta.permission: 'receiving:write'`.
- Menu:
  - "Compras" for `purchase:write`;
  - "Receber itens" for `receiving:write` without `purchase:write`.
  The menu order is the design document's.
- `features/pedidos/`:
  - the item row reads `orderPurchases` through the pedidos store and shows the C-number (a link for `purchase:write`) and "Prazo do fornecedor";
  - it shows "Ver em Compras" or "Receber itens", and only "Marcar como instalada" among the moves of a bought item;
  - the delay banner shows "Falar com o fornecedor" when `supplierChatUrl` is there;
  - `historyText.ts` writes the purchasing entries of `HistoryEntry`.
- `features/hoje/`: Hoje calls `purchasingToday` only for `purchase:write`. It adds the "Compras a enviar" card and puts the entries at the top of "Precisa de você", in the API's order.
- **Workshop sizes (5.14):** `packages/ui/src/tokens.css` gains a scoped tier, `.enc-workshop`, with 18 px text, `--enc-control-height: 56px` and a 64 px primary button. Receber itens sets it on its root. `web/CLAUDE.md` names it under "Product and design rules" (sizes) and "Phone layouts".
- **Phone:**
  - Enviadas, Fornecedores and the pedido de compra's lines are `enc-cards` tables;
  - `PromisedDateDialog`, `PaidPricesDialog`, `UndoDialog` and `SupplierDrawer` are `BaseDialog` sheets with `enc-sheet-actions`;
  - the A comprar boxes are `enc-check` labels.
- Wording: `docs/product/design-document.md`, "Milestone 5 screens". The Fornecedores table has "Fornecedor" and "WhatsApp"; the drawer lists the supplier's items. "{n} itens" counts entries, not summed quantities. Costs are shown as "Custo (por unidade)" and "Preço pago (por unidade)".

### 6.3 Web tests

- Unit tests with MSW per screen, as M4's.
- Phone layouts at 390 × 844. Receber itens at workshop sizes: 18 px text, 56 px targets, a 64 px main button.
- `web/e2e/compras.spec.ts` and `phone-compras.spec.ts` against the real stack (5.16).
- `pedidos.spec.ts` and `phone-pedidos.spec.ts` receive items through Receber itens, not the order screen.
