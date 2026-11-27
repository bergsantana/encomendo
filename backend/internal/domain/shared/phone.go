package shared

// ValidWhatsAppPhone reports whether p is a Brazilian number as wa.me links
// need it: digits only, "55", a DDD of two digits that start with 1 to 9,
// then 8 or 9 digits. It is the rule ^55[1-9]{2}[0-9]{8,9}$ of
// docs/api/openapi.yaml (SupplierPhone and the store's whatsappPhone). It
// never trims or reformats p: a number that needs cleaning is refused, so
// every stored number works in a link.
func ValidWhatsAppPhone(p string) bool {
	if len(p) != 12 && len(p) != 13 || p[0] != '5' || p[1] != '5' || p[2] == '0' || p[3] == '0' {
		return false
	}
	for i := range len(p) {
		if p[i] < '0' || p[i] > '9' {
			return false
		}
	}
	return true
}
