package purchasing

import "fmt"

// Number is a supplier order's number within its store, counted from 1. Each
// store has its own counter, as orders do (ADR 0030, ADR 0031).
type Number int

// String returns the number as staff see it: "C-" and at least four digits,
// such as C-0012 or C-12345. The zero Number, a supplier order not sent yet,
// reads "C-0000"; only TalkMessage, for an item not sent yet, takes a zero
// Number, and it leaves it out instead of printing it.
func (n Number) String() string { return fmt.Sprintf("C-%04d", int(n)) }

// Valid reports whether n is a number a store can have issued.
func (n Number) Valid() bool { return n >= 1 }
