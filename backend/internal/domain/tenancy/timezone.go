package tenancy

import "time"

// TimeZone is the IANA zone a store's "today" is counted in. Brazil has no
// daylight saving time since 2019, so one zone per offset is enough (ADR
// 0030); the four values are the ones 0015_settings_time_zone allows.
type TimeZone string

// The zones a store may choose.
const (
	// ZoneNoronha is UTC-2 (Fernando de Noronha).
	ZoneNoronha TimeZone = "America/Noronha"
	// ZoneSaoPaulo is UTC-3, Brasília time, the default.
	ZoneSaoPaulo TimeZone = "America/Sao_Paulo"
	// ZoneManaus is UTC-4 (Amazonas, Mato Grosso, and others).
	ZoneManaus TimeZone = "America/Manaus"
	// ZoneRioBranco is UTC-5 (Acre).
	ZoneRioBranco TimeZone = "America/Rio_Branco"
)

// Zones returns the allowed zones: Noronha, São Paulo, Manaus, Rio Branco.
func Zones() []TimeZone {
	return []TimeZone{ZoneNoronha, ZoneSaoPaulo, ZoneManaus, ZoneRioBranco}
}

// Valid reports whether z is one of Zones.
func (z TimeZone) Valid() bool {
	switch z {
	case ZoneNoronha, ZoneSaoPaulo, ZoneManaus, ZoneRioBranco:
		return true
	}
	return false
}

// Location loads the zone's rules with time.LoadLocation. It returns
// ErrInvalidTimeZone for a zone that is not Valid, and an error for a host
// without zone data; cmd/api embeds time/tzdata so the second cannot happen
// in production.
func (z TimeZone) Location() (*time.Location, error) {
	if !z.Valid() {
		return nil, ErrInvalidTimeZone
	}
	return time.LoadLocation(string(z))
}
