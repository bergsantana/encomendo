package handler_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler/dto"
	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
)

// TestHistoryEntry_CarriesThePedidoDeCompra: a purchasing move's entry
// carries its C-number as ref, as the contract says, and an entry with none
// has no ref key (found by the milestone 5 security pass).
func TestHistoryEntry_CarriesThePedidoDeCompra(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC)
	sent, err := json.Marshal(dto.NewHistoryEntry(apporder.HistoryView{Kind: order.HistoryItem, At: at,
		ItemFrom: order.ItemToBuy, ItemTo: order.ItemSentToSupplier, Ref: "C-0012"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sent), `"ref":"C-0012"`) {
		t.Errorf("entry = %s, want ref C-0012", sent)
	}
	plain, err := json.Marshal(dto.NewHistoryEntry(apporder.HistoryView{Kind: order.HistoryCreated, At: at}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), `"ref"`) {
		t.Errorf("entry with no ref = %s, want no ref key", plain)
	}
}
