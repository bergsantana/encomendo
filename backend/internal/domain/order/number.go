package order

import "fmt"

// Number is an order's number within its store, counted from 1. Each store
// has its own counter (ADR 0030).
type Number int

// String returns the number as staff and clients see it: "P-" and at least
// four digits, such as P-0231 or P-12345.
func (n Number) String() string { return fmt.Sprintf("P-%04d", int(n)) }

// Valid reports whether n is a number a store can have issued.
func (n Number) Valid() bool { return n >= 1 }
