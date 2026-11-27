package tenancy

import (
	"errors"
	"testing"
	"time"
)

func TestZones_AreTheFourBrazilianOffsetsInOrder(t *testing.T) {
	t.Parallel()

	want := []TimeZone{"America/Noronha", "America/Sao_Paulo", "America/Manaus", "America/Rio_Branco"}
	got := Zones()
	if len(got) != len(want) {
		t.Fatalf("Zones = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Zones[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	got[0] = "changed"
	if Zones()[0] != ZoneNoronha {
		t.Error("Zones returned a shared slice")
	}
}

func TestTimeZone_Valid(t *testing.T) {
	t.Parallel()

	for _, z := range Zones() {
		if !z.Valid() {
			t.Errorf("%q is not valid", z)
		}
	}
	for _, z := range []TimeZone{"", "UTC", "America/New_York", "america/sao_paulo", "America/Sao_Paulo "} {
		if z.Valid() {
			t.Errorf("%q is valid", z)
		}
	}
}

func TestTimeZone_Location(t *testing.T) {
	t.Parallel()

	// Brazil has no daylight saving time, so each zone has one fixed offset.
	offsets := map[TimeZone]int{ZoneNoronha: -2, ZoneSaoPaulo: -3, ZoneManaus: -4, ZoneRioBranco: -5}
	instant := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	for z, hours := range offsets {
		loc, err := z.Location()
		if err != nil {
			t.Fatalf("%s: %v", z, err)
		}
		if _, off := instant.In(loc).Zone(); off != hours*3600 {
			t.Errorf("%s offset = %d s, want %d h", z, off, hours)
		}
	}
	if _, err := TimeZone("UTC").Location(); !errors.Is(err, ErrInvalidTimeZone) {
		t.Errorf("an invalid zone: %v, want ErrInvalidTimeZone", err)
	}
}
