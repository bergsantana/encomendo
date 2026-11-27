package workflow

// StatusKey names a status inside one workflow, such as "purchasing". Keys
// are data: code never branches on them, only on the status's Phase.
type StatusKey string

// keyCancelled is the reserved key of the implicit Cancelled status.
const keyCancelled StatusKey = "cancelled"

// Status is one step of a workflow: how staff and the client see it, and the
// phase the rules treat it as.
type Status struct {
	Key StatusKey
	// Label is what staff see, in pt-BR.
	Label string
	// ClientLabel is what the store page and the client area show, in pt-BR.
	// It never names suppliers or purchases.
	ClientLabel string
	Phase       Phase
}

// Cancelled is the implicit status every workflow ends in when an order is
// cancelled. It is never a row; its key is reserved.
var Cancelled = Status{Key: keyCancelled, Label: "Cancelado", ClientLabel: "Pedido cancelado", Phase: PhaseCancelled}
