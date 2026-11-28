package models_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres/models"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

func id(n byte) shared.ID { return shared.NewV7(1_700_000_000_000, [10]byte{n}) }

// Every history kind round-trips through its stored row with every field it
// carries (go-reviewer on 4.12).
func TestOrderHistory_EveryKindRoundTrips(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 12, 3, 14, 30, 0, 0, time.UTC)
	money, err := shared.NewMoney(12_345, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	zero, _ := shared.NewMoney(0, "BRL")
	d1, _ := shared.ParseCivilDate("2026-12-21")
	d2, _ := shared.ParseCivilDate("2027-01-05")
	from := workflow.Status{Key: "purchasing", Label: "Compras", ClientLabel: "Pedido confirmado", Phase: workflow.PhaseSourcing}
	to := workflow.Status{Key: "ready", Label: "Pronto", ClientLabel: "Pronto", Phase: workflow.PhaseReady}
	by := shared.UserID(id(1))
	item := shared.OrderItemID(id(2))
	pay := shared.PaymentID(id(3))
	entries := []order.HistoryEntry{
		{Kind: order.HistoryCreated, By: by, At: at, To: from},
		{Kind: order.HistoryStatus, By: by, At: at, From: from, To: to, Handover: order.HandoverPickup},
		{Kind: order.HistoryItem, By: by, At: at, Item: item, ItemFrom: order.ItemInTransit, ItemTo: order.ItemReceived, Check: order.CheckOK},
		{Kind: order.HistoryBackupSupplier, By: by, At: at, Item: item},
		{Kind: order.HistoryPayment, By: by, At: at, Payment: pay, Amount: money, PaymentKind: order.PaymentDeposit, PaymentMethod: order.MethodPix},
		{Kind: order.HistoryPaymentVoided, By: by, At: at, Payment: pay, Amount: zero, Reason: "Pix não caiu"},
		{Kind: order.HistoryReleased, By: by, At: at},
		{Kind: order.HistoryDate, By: by, At: at, OldDate: d1, NewDate: d2, Reason: "Fornecedor atrasou"},
		{Kind: order.HistoryChanged, By: by, At: at, OldDate: d1, NewDate: d2, Amount: money, CustomerApproved: true},
		{Kind: order.HistoryTrackingLink, At: at},
		{Kind: order.HistoryCancelled, By: by, At: at, From: from, To: workflow.Cancelled, Reason: "Cliente desistiu"},
	}
	for _, e := range entries {
		t.Run(string(e.Kind), func(t *testing.T) {
			t.Parallel()
			row, err := models.OrderHistoryFromEntry(shared.TenantID(id(4)), id(5), shared.OrderID(id(6)), e)
			if err != nil {
				t.Fatal(err)
			}
			got, err := models.OrderHistoryRead{ID: row.ID, Kind: row.Kind, ByUser: row.ByUser, At: row.At, Detail: row.Detail}.ToEntry()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, e) {
				t.Errorf("round trip\n got %+v\nwant %+v", got, e)
			}
		})
	}
}
