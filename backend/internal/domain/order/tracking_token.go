package order

import (
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// RotateTracking replaces the order's tracking token with the one hashed as
// h, on behalf of by. The previous link stops working at once, because
// only the hash is stored: a link can never be shown again, so every send
// issues a new one (ADR 0030).
func (o *Order) RotateTracking(h shared.SecretHash, by shared.Principal, now time.Time) {
	o.tracking = h
	o.addHistory(HistoryEntry{Kind: HistoryTrackingLink, By: by.UserID(), At: now})
}
