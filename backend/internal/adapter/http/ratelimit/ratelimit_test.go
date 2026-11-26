package ratelimit_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/ratelimit"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestClientIP_Of(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		remote string
		xff    []string
		want   string
	}{
		{name: "a direct client is its peer", remote: "203.0.113.7:51000", want: "203.0.113.7"},
		{name: "a direct client cannot pick its address", remote: "203.0.113.7:51000", xff: []string{"198.51.100.1"}, want: "203.0.113.7"},
		// Caddy → api: Caddy writes the client.
		{name: "one trusted hop", remote: "172.18.0.5:40000", xff: []string{"203.0.113.7"}, want: "203.0.113.7"},
		// Caddy → nginx → api: nginx appends Caddy.
		{name: "two trusted hops", remote: "172.18.0.6:40000", xff: []string{"203.0.113.7, 172.18.0.5"}, want: "203.0.113.7"},
		{name: "a spoofed leftmost hop is ignored", remote: "172.18.0.6:40000", xff: []string{"198.51.100.1, 203.0.113.7, 172.18.0.5"}, want: "203.0.113.7"},
		{name: "a spoofed hop that looks trusted is skipped, not taken", remote: "172.18.0.6:40000", xff: []string{"10.0.0.1, 203.0.113.7"}, want: "203.0.113.7"},
		{name: "header lines read in order", remote: "172.18.0.6:40000", xff: []string{"198.51.100.1", "203.0.113.7, 172.18.0.5"}, want: "203.0.113.7"},
		{name: "spaces around entries", remote: "172.18.0.6:40000", xff: []string{" 203.0.113.7 ,  172.18.0.5 "}, want: "203.0.113.7"},
		{name: "trusted hops only stop at the last", remote: "127.0.0.1:40000", xff: []string{"10.1.2.3, 172.18.0.5"}, want: "10.1.2.3"},
		{name: "no header behind a proxy", remote: "172.18.0.6:40000", want: "172.18.0.6"},
		{name: "a malformed entry stops at the hop after it", remote: "172.18.0.6:40000", xff: []string{"203.0.113.7, evil, 172.18.0.5"}, want: "172.18.0.5"},
		{name: "a malformed header stops at the peer", remote: "172.18.0.6:40000", xff: []string{"<script>"}, want: "172.18.0.6"},
		{name: "an empty entry stops the walk", remote: "172.18.0.6:40000", xff: []string{"203.0.113.7,,172.18.0.5"}, want: "172.18.0.5"},
		{name: "IPv6 client", remote: "[fd00::5]:40000", xff: []string{"2001:db8:1:2::7"}, want: "2001:db8:1:2::7"},
		{name: "IPv4-mapped IPv6 is IPv4", remote: "[::ffff:172.18.0.6]:40000", xff: []string{"::ffff:203.0.113.7"}, want: "203.0.113.7"},
		{name: "a zone is dropped", remote: "[fe80::1%eth0]:40000", want: "fe80::1"},
		// Neither Caddy nor nginx writes ports; an entry with one is not ours.
		{name: "an entry with a port stops the walk", remote: "172.18.0.6:40000", xff: []string{"203.0.113.7:443"}, want: "172.18.0.6"},
		{name: "a bracketed entry stops the walk", remote: "172.18.0.6:40000", xff: []string{"[2001:db8::1]:443"}, want: "172.18.0.6"},
	}
	ip := ratelimit.NewClientIP(ratelimit.DefaultTrustedProxies())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
			r.RemoteAddr = tt.remote
			for _, v := range tt.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			got, ok := ip.Of(r)
			if !ok || got.String() != tt.want {
				t.Errorf("Of = %v, %v; want %s", got, ok, tt.want)
			}
		})
	}
}

func TestClientIP_UnreadablePeer(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	r.RemoteAddr = "not an address"
	ip := ratelimit.NewClientIP(ratelimit.DefaultTrustedProxies())
	if _, ok := ip.Of(r); ok {
		t.Error("Of read an address from nothing")
	}
	if v := ip.Value(r); v != "" {
		t.Errorf("Value = %q, want empty", v)
	}
}

// An IPv6 subscriber holds a whole /64, so every address in it shares a
// count (ADR 0021).
func TestClientIP_ValueKeysIPv6OnItsSlash64(t *testing.T) {
	t.Parallel()

	ip := ratelimit.NewClientIP(nil)
	value := func(remote string) string {
		r := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		r.RemoteAddr = remote
		return ip.Value(r)
	}
	if a, b := value("[2001:db8:1:2::7]:1"), value("[2001:db8:1:2:ffff::1]:1"); a != b || a != "2001:db8:1:2::/64" {
		t.Errorf("one /64 gives %q and %q, want both 2001:db8:1:2::/64", a, b)
	}
	if a, b := value("[2001:db8:1:2::7]:1"), value("[2001:db8:1:3::7]:1"); a == b {
		t.Errorf("two /64s share %q", a)
	}
	if v := value("203.0.113.7:1"); v != "203.0.113.7" {
		t.Errorf("IPv4 value = %q", v)
	}
	behind := ratelimit.NewClientIP(ratelimit.DefaultTrustedProxies())
	r := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	r.RemoteAddr = "172.18.0.6:40000"
	r.Header.Set("X-Forwarded-For", "2001:db8:1:2::7, 172.18.0.5")
	if v := behind.Value(r); v != "2001:db8:1:2::/64" {
		t.Errorf("IPv6 client behind the proxies = %q, want its /64", v)
	}
}

func TestClientIP_TrustsOnlyItsList(t *testing.T) {
	t.Parallel()

	ip := ratelimit.NewClientIP([]netip.Prefix{netip.MustParsePrefix("172.18.0.0/16")})
	r := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	r.RemoteAddr = "10.0.0.9:1"
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	if got, _ := ip.Of(r); got.String() != "10.0.0.9" {
		t.Errorf("an untrusted peer's header was read: %v", got)
	}
}

// TestRules_MatchTheADR pins every number to the table in ADR 0021, so a
// changed limit changes the ADR too.
func TestRules_MatchTheADR(t *testing.T) {
	t.Parallel()

	want := []string{
		"signup ip 10 1h0m0s", "signup email 5 1h0m0s",
		"login ip 60 10m0s", "login email 10 10m0s",
		"refresh ip 300 10m0s",
		"password-reset ip 10 1h0m0s", "password-reset email 5 1h0m0s",
		"password-reset-confirm ip 30 10m0s", "password-reset-confirm email 10 10m0s",
		"invitation-preview ip 60 10m0s", "invitation-accept ip 20 10m0s",
		"create-tenant user 5 24h0m0s",
		"invite store 20 24h0m0s", "invite user 50 24h0m0s", "invite store-email 3 24h0m0s",
	}
	rules := ratelimit.Rules()
	if len(rules) != len(want) {
		t.Fatalf("%d rules, want %d", len(rules), len(want))
	}
	for i, r := range rules {
		if got := fmt.Sprintf("%s %s %d %v", r.Name, r.Kind, r.Limit, r.Window); got != want[i] {
			t.Errorf("rule %d = %s, want %s", i, got, want[i])
		}
		if strings.Contains(r.Name, ":") {
			t.Errorf("rule name %q has a colon, which would break keys", r.Name)
		}
	}
}

func TestRule_Key(t *testing.T) {
	t.Parallel()

	if got := ratelimit.LogInByIP.Key("abc"); got != "g:rl:login:ip:abc" {
		t.Errorf("Key = %q", got)
	}
}

func key(b byte) []byte { return bytes.Repeat([]byte{b}, ratelimit.MinKeyLength) }

func email(t *testing.T, s string) identity.Email {
	t.Helper()
	e, err := identity.ParseEmail(s)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestNewPseudonyms_RefusesAShortKey(t *testing.T) {
	t.Parallel()

	for _, k := range [][]byte{nil, {}, key(1)[:ratelimit.MinKeyLength-1]} {
		if _, err := ratelimit.NewPseudonyms(k); err == nil {
			t.Errorf("a %d-byte key was accepted", len(k))
		}
	}
}

func TestPseudonyms(t *testing.T) {
	t.Parallel()

	p, err := ratelimit.NewPseudonyms(key(1))
	if err != nil {
		t.Fatal(err)
	}
	other, err := ratelimit.NewPseudonyms(key(2))
	if err != nil {
		t.Fatal(err)
	}
	v := p.Email(email(t, "Ana@Example.com"))
	if v != p.Email(email(t, "ana@example.com")) {
		t.Error("changing the case of an email changed its value")
	}
	if len(v) != 64 || strings.Contains(v, "ana") {
		t.Errorf("Email = %q, want 64 hex digits and no trace of the email", v)
	}
	if v == other.Email(email(t, "ana@example.com")) {
		t.Error("two keys give the same value: the email is not keyed")
	}
	if v == p.Email(email(t, "bia@example.com")) {
		t.Error("two emails share a value")
	}
	if ip := p.IP("203.0.113.7"); len(ip) != 64 || strings.Contains(ip, "203") || ip == p.IP("203.0.113.8") {
		t.Errorf("IP = %q, want a keyed value with no trace of the address", ip)
	}
	if p.IP("x") == p.Email(email(t, "x@example.com")) {
		t.Error("an address and an email can share a value")
	}
	id, err := shared.ParseID("01936d1e-8b5a-7c2e-9f3a-1b2c3d4e5f60")
	if err != nil {
		t.Fatal(err)
	}
	store := shared.TenantID(id)
	if got := p.StoreEmail(store, email(t, "ANA@example.com")); got != store.String()+":"+v {
		t.Errorf("StoreEmail = %q", got)
	}
}
