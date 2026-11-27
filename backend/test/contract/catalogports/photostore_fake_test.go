package catalogports

import (
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/app/catalog/fake"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
)

func TestFakePhotoStore_Contract(t *testing.T) {
	RunPhotoStoreContract(t, func(*testing.T) PhotoStoreHarness {
		clock := sharedfake.NewClock(time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC))
		store := fake.NewStorage(clock)
		return PhotoStoreHarness{
			Store: store,
			Upload: func(_ *testing.T, key, contentType string, data []byte) {
				store.Upload(key, contentType, data)
			},
			Read:  func(_ *testing.T, key string) ([]byte, string, bool) { return store.Promoted(key) },
			NewID: sharedfake.NewIDs(clock).NewID,
		}
	})
}
