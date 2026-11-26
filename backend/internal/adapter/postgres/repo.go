package postgres

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Row locks, for the reads the ports ask to lock.
var (
	forUpdate = clause.Locking{Strength: clause.LockingStrengthUpdate}
	forShare  = clause.Locking{Strength: clause.LockingStrengthShare}
)

// wrap adds what the repository was doing to err and maps it like every
// repository error (errors.go). A nil error stays nil.
func wrap(doing string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", doing, mapError(err))
}

// affected reports whether a conditional update changed a row.
func affected(res *gorm.DB, doing string) (bool, error) {
	if res.Error != nil {
		return false, wrap(doing, res.Error)
	}
	return res.RowsAffected > 0, nil
}

// notFound is the error of a write that matched no row.
func notFound(doing string) error {
	return fmt.Errorf("%s: %w", doing, shared.ErrNotFound)
}

// errBadCursor is what a cursor the repository did not issue answers: a
// ValidationError on "cursor", so the API says 400 (backend/CLAUDE.md).
func errBadCursor() error {
	return shared.NewValidationError(shared.FieldError{Field: "cursor", Message: "Cursor inválido."})
}

// maxCursorMicros is the last microsecond of the year 9999.
const maxCursorMicros = 253402300799999999

// position is where a keyset page ends: the created_at, in microseconds,
// and the id of its last row. Postgres keeps microseconds, so the cursor
// compares exactly.
type position struct {
	micros int64
	id     string
}

func (p position) cursor() string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(p.micros, 10) + "." + p.id))
}

func positionOf(created time.Time, id string) position {
	return position{micros: created.UnixMicro(), id: id}
}

// parseCursor reads what position.cursor wrote; the empty cursor is the
// start. Anything else is errBadCursor.
func parseCursor(c string) (position, bool, error) {
	if c == "" {
		return position{}, false, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return position{}, false, errBadCursor()
	}
	micros, id, ok := strings.Cut(string(raw), ".")
	if !ok {
		return position{}, false, errBadCursor()
	}
	n, err := strconv.ParseInt(micros, 10, 64)
	// Only what position.cursor wrote: canonical digits, and a time Postgres
	// can compare (years 1970 to 9999), or the query itself would fail.
	if _, perr := shared.ParseID(id); err != nil || perr != nil || strconv.FormatInt(n, 10) != micros || n < 0 || n > maxCursorMicros {
		return position{}, false, errBadCursor()
	}
	return position{micros: n, id: id}, true, nil
}

func (p position) time() time.Time { return time.UnixMicro(p.micros).UTC() }
