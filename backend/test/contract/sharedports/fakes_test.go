package sharedports

import (
	"testing"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var start = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

func TestFakeCache_Contract(t *testing.T) {
	RunCacheContract(t, func(*testing.T) CacheHarness {
		clock := sharedfake.NewClock(start)
		return CacheHarness{
			Cache:  sharedfake.NewCache(clock),
			Elapse: func(_ *testing.T, d time.Duration) { clock.Advance(d) },
		}
	})
}

func TestFakeTxManager_Contract(t *testing.T) {
	RunTxManagerContract(t, func(*testing.T) appshared.TxManager { return sharedfake.NewTxManager() })
}

func TestFakeEvents_Contract(t *testing.T) {
	RunEventPublisherContract(t, func(*testing.T) EventsHarness {
		events := sharedfake.NewEvents()
		return EventsHarness{
			Tx:        sharedfake.NewTxManager(),
			Publisher: events,
			TenantID:  shared.TenantID(shared.NewV7(start.UnixMilli(), [10]byte{0xe})),
			Committed: func(t *testing.T) []StoredEvent {
				var stored []StoredEvent
				for _, r := range events.Records() {
					stored = append(stored, StoredEvent{Name: r.Name, Payload: r.Payload})
				}
				return stored
			},
		}
	})
}

func TestFakeClock_Contract(t *testing.T) {
	RunClockContract(t, func(*testing.T) appshared.Clock { return sharedfake.NewClock(start) })
}

func TestFakeIDs_Contract(t *testing.T) {
	RunIDGeneratorContract(t, func(*testing.T) appshared.IDGenerator {
		return sharedfake.NewIDs(sharedfake.NewClock(start))
	})
}
