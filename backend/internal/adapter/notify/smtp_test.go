package notify

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"strings"
	"sync"
	"testing"
	"time"

	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// server is a minimal SMTP server that keeps what it is sent. reject makes
// it answer the recipient with 550 and a text that quotes the address, as
// real servers do.
type server struct {
	addr   string
	mu     sync.Mutex
	mails  []sent
	reject bool
	hang   bool
	// offerAuth advertises AUTH PLAIN and records the credentials sent;
	// offerTLS advertises STARTTLS, which the server then refuses (454).
	offerAuth, offerTLS bool
	auths               []string
	release             chan struct{}
}

type sent struct{ from, to, data string }

func startServer(t *testing.T, configure func(*server)) *server {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &server{addr: l.Addr().String(), release: make(chan struct{})}
	if configure != nil {
		configure(s)
	}
	t.Cleanup(func() { close(s.release); _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()
	return s
}

func (s *server) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	if s.hang {
		<-s.release
		return
	}
	r := bufio.NewReader(c)
	say := func(line string) { _, _ = fmt.Fprintf(c, "%s\r\n", line) }
	say("220 localhost ESMTP")
	var m sent
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"):
			switch {
			case s.offerAuth:
				say("250-localhost")
				if s.offerTLS {
					say("250-STARTTLS")
				}
				say("250 AUTH PLAIN")
			case s.offerTLS:
				say("250-localhost")
				say("250 STARTTLS")
			default:
				say("250 localhost")
			}
		case cmd == "STARTTLS":
			say("454 4.7.0 TLS not available")
		case strings.HasPrefix(cmd, "AUTH PLAIN"):
			s.mu.Lock()
			s.auths = append(s.auths, strings.TrimSpace(line[len("AUTH PLAIN "):]))
			s.mu.Unlock()
			say("235 ok")
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			m.from = strings.Trim(strings.TrimSpace(line[len("MAIL FROM:"):]), "<>")
			say("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			m.to = strings.Trim(strings.TrimSpace(line[len("RCPT TO:"):]), "<>")
			if s.reject {
				say("550 5.1.1 <" + m.to + "> user unknown")
				continue
			}
			say("250 ok")
		case cmd == "DATA":
			say("354 go on")
			var data bytes.Buffer
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				data.WriteString(l)
			}
			m.data = data.String()
			s.mu.Lock()
			s.mails = append(s.mails, m)
			s.mu.Unlock()
			say("250 queued")
		case cmd == "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

func (s *server) received() []sent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]sent(nil), s.mails...)
}

func newMailer(t *testing.T, addr string, log *slog.Logger) *SMTP {
	t.Helper()
	m, err := NewSMTP(SMTPConfig{
		Addr: addr, From: "nao-responda@encomendo.com.br", FromName: "Encomendo",
		AdminOrigin: "https://admin.encomendo.com.br", Timeout: 2 * time.Second,
	}, log)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func mustEmail(t *testing.T, s string) identity.Email {
	t.Helper()
	e, err := identity.ParseEmail(s)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func invitation(t *testing.T) (apptenancy.InvitationEmail, string) {
	t.Helper()
	tok := tenancy.NewInvitationToken(shared.TenantID(shared.NewV7(1_700_000_000_000, [10]byte{3})), shared.Secret{7})
	return apptenancy.InvitationEmail{
		To: mustEmail(t, "carla@ex.com"), Store: "Loja da Ana", InvitedBy: "Ana Souza", Role: shared.RoleStaff,
		Token: tok, ExpiresAt: time.Date(2026, 12, 2, 12, 0, 0, 0, time.UTC),
	}, tok.Reveal()
}

// parse reads a received message: headers, and the body decoded.
func parse(t *testing.T, raw string) (mail.Header, string) {
	t.Helper()
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("not an email: %v\n%s", err, raw)
	}
	body, err := io.ReadAll(quotedprintable.NewReader(msg.Body))
	if err != nil {
		t.Fatal(err)
	}
	return msg.Header, string(body)
}

func TestSendInvitation_SendsTheLinkInPortuguese(t *testing.T) {
	t.Parallel()
	srv := startServer(t, nil)
	m := newMailer(t, srv.addr, slog.New(slog.DiscardHandler))
	e, token := invitation(t)

	if err := m.SendInvitation(t.Context(), e); err != nil {
		t.Fatalf("SendInvitation: %v", err)
	}

	got := srv.received()
	if len(got) != 1 || got[0].from != "nao-responda@encomendo.com.br" || got[0].to != "carla@ex.com" {
		t.Fatalf("received %+v; want one mail from the sender to carla@ex.com", got)
	}
	h, body := parse(t, got[0].data)
	subject, err := new(mime.WordDecoder).DecodeHeader(h.Get("Subject"))
	if err != nil || subject != "Convite para a equipe de Loja da Ana" {
		t.Errorf("Subject = %q, %v", subject, err)
	}
	for _, want := range []string{
		"Ana Souza convidou você para a equipe da loja Loja da Ana no Encomendo, como funcionário.",
		"https://admin.encomendo.com.br/convite#token=" + token,
		"O convite vale até 02/12/2026.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the body lacks %q:\n%s", want, body)
		}
	}
	if !strings.HasPrefix(h.Get("Content-Type"), "text/plain; charset=UTF-8") || h.Get("Message-Id") == "" || h.Get("Date") == "" {
		t.Errorf("headers %v; want plain UTF-8 text with a Message-ID and a Date", h)
	}
}

// TestSendInvitation_AStoreNameCannotAddAHeader: names are validated as
// printable, but the mailer must not rely on it for the headers.
func TestSendInvitation_AStoreNameCannotAddAHeader(t *testing.T) {
	t.Parallel()
	srv := startServer(t, nil)
	m := newMailer(t, srv.addr, slog.New(slog.DiscardHandler))
	e, _ := invitation(t)
	e.Store = "Loja\r\nBcc: alvo@ex.com"

	if err := m.SendInvitation(t.Context(), e); err != nil {
		t.Fatal(err)
	}

	h, _ := parse(t, srv.received()[0].data)
	if h.Get("Bcc") != "" {
		t.Errorf("a store name added a Bcc header: %v", h)
	}
}

// TestSendInvitation_FailureNeverCarriesTheAddressOrTheLink: the error goes
// to the log and to the 503 detail (ADR 0011), and a server's reply quotes
// the address.
func TestSendInvitation_FailureNeverCarriesTheAddressOrTheLink(t *testing.T) {
	t.Parallel()
	e, token := invitation(t)

	for name, addr := range map[string]func(t *testing.T) string{
		"the recipient is refused": func(t *testing.T) string {
			return startServer(t, func(s *server) { s.reject = true }).addr
		},
		"nothing listens": func(t *testing.T) string {
			l, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			a := l.Addr().String()
			_ = l.Close()
			return a
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := newMailer(t, addr(t), slog.New(slog.DiscardHandler))
			err := m.SendInvitation(t.Context(), e)
			if !errors.Is(err, ErrDelivery) {
				t.Fatalf("SendInvitation = %v; want ErrDelivery", err)
			}
			for _, secret := range []string{"carla@ex.com", token, "Loja da Ana"} {
				if strings.Contains(err.Error(), secret) {
					t.Errorf("the error %q carries %q", err, secret)
				}
			}
		})
	}
}

func TestSendInvitation_GivesUpAfterItsTimeout(t *testing.T) {
	t.Parallel()
	srv := startServer(t, func(s *server) { s.hang = true })
	m, err := NewSMTP(SMTPConfig{
		Addr: srv.addr, From: "nao-responda@encomendo.com.br", AdminOrigin: "https://admin.encomendo.com.br",
		Timeout: 200 * time.Millisecond,
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	e, _ := invitation(t)

	start := time.Now()
	err = m.SendInvitation(t.Context(), e)

	if !errors.Is(err, ErrDelivery) || time.Since(start) > 3*time.Second {
		t.Errorf("SendInvitation = %v after %v; want ErrDelivery after about 200ms", err, time.Since(start))
	}
}

func TestSendResetCode_ReturnsAtOnceAndSendsTheCode(t *testing.T) {
	t.Parallel()
	srv := startServer(t, nil)
	m := newMailer(t, srv.addr, slog.New(slog.DiscardHandler))
	code, err := identity.ParseResetCode("483920")
	if err != nil {
		t.Fatal(err)
	}
	name, _ := identity.ParsePersonName("Ana Souza")

	m.SendResetCode(t.Context(), appidentity.ResetCodeEmail{To: mustEmail(t, "ana@ex.com"), Name: name, Code: code})
	m.Wait()

	got := srv.received()
	if len(got) != 1 || got[0].to != "ana@ex.com" {
		t.Fatalf("received %+v; want one mail to ana@ex.com", got)
	}
	_, body := parse(t, got[0].data)
	for _, want := range []string{"Olá, Ana Souza.", "483920", "Ele vale por 10 minutos."} {
		if !strings.Contains(body, want) {
			t.Errorf("the body lacks %q:\n%s", want, body)
		}
	}
}

// TestSendResetCode_DoesNotWaitForTheServer: the request's answer must not
// wait for SMTP, or its timing would tell whether the account exists.
func TestSendResetCode_DoesNotWaitForTheServer(t *testing.T) {
	t.Parallel()
	srv := startServer(t, func(s *server) { s.hang = true })
	m := newMailer(t, srv.addr, slog.New(slog.DiscardHandler))
	code, _ := identity.ParseResetCode("483920")
	name, _ := identity.ParsePersonName("Ana Souza")

	start := time.Now()
	m.SendResetCode(t.Context(), appidentity.ResetCodeEmail{To: mustEmail(t, "ana@ex.com"), Name: name, Code: code})

	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("SendResetCode took %v against a server that does not answer", elapsed)
	}
	m.Wait()
}

func TestSendResetCode_LogsAFailureWithoutTheAddressOrTheCode(t *testing.T) {
	t.Parallel()
	srv := startServer(t, func(s *server) { s.reject = true })
	var logs bytes.Buffer
	m := newMailer(t, srv.addr, slog.New(slog.NewTextHandler(&logs, nil)))
	code, _ := identity.ParseResetCode("483920")
	name, _ := identity.ParsePersonName("Ana Souza")

	m.SendResetCode(t.Context(), appidentity.ResetCodeEmail{To: mustEmail(t, "ana@ex.com"), Name: name, Code: code})
	m.Wait()

	out := logs.String()
	if !strings.Contains(out, "reset code email not sent") {
		t.Fatalf("the failure was not logged: %q", out)
	}
	for _, secret := range []string{"ana@ex.com", "483920"} {
		if strings.Contains(out, secret) {
			t.Errorf("the log carries %q: %s", secret, out)
		}
	}
}

func TestSendResetCode_SurvivesTheEndOfTheRequest(t *testing.T) {
	t.Parallel()
	srv := startServer(t, nil)
	m := newMailer(t, srv.addr, slog.New(slog.DiscardHandler))
	code, _ := identity.ParseResetCode("483920")
	name, _ := identity.ParsePersonName("Ana Souza")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	m.SendResetCode(ctx, appidentity.ResetCodeEmail{To: mustEmail(t, "ana@ex.com"), Name: name, Code: code})
	m.Wait()

	if n := len(srv.received()); n != 1 {
		t.Errorf("%d mails sent after the request ended; want 1", n)
	}
}

func TestNewSMTP_RefusesWhatItCannotUse(t *testing.T) {
	t.Parallel()
	ok := SMTPConfig{Addr: "localhost:1025", From: "a@encomendo.com.br", AdminOrigin: "https://admin.encomendo.com.br", Timeout: time.Second}

	for name, edit := range map[string]func(*SMTPConfig){
		"a sender that is not an address":   func(c *SMTPConfig) { c.From = "não é e-mail" },
		"a server address without a port":   func(c *SMTPConfig) { c.Addr = "localhost" },
		"no timeout":                        func(c *SMTPConfig) { c.Timeout = 0 },
		"an admin origin with a slash":      func(c *SMTPConfig) { c.AdminOrigin += "/" },
		"an admin origin that is not a URL": func(c *SMTPConfig) { c.AdminOrigin = "admin.encomendo.com.br" },
	} {
		cfg := ok
		edit(&cfg)
		if m, err := NewSMTP(cfg, slog.New(slog.DiscardHandler)); err == nil || m != nil {
			t.Errorf("%s: NewSMTP = %v, %v; want an error", name, m, err)
		}
	}
}

// TestSendInvitation_SignsInWhenItHasCredentials: the conversation has an
// AUTH step, which production will use. net/smtp allows PlainAuth without
// TLS only to localhost, which is where this server is.
func TestSendInvitation_SignsInWhenItHasCredentials(t *testing.T) {
	t.Parallel()
	srv := startServer(t, func(s *server) { s.offerAuth = true })
	m, err := NewSMTP(SMTPConfig{
		Addr: srv.addr, From: "nao-responda@encomendo.com.br", AdminOrigin: "https://admin.encomendo.com.br",
		Timeout: 2 * time.Second, Username: "api", Password: "senha-do-smtp",
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	e, _ := invitation(t)

	if err := m.SendInvitation(t.Context(), e); err != nil {
		t.Fatalf("SendInvitation: %v", err)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	want := base64.StdEncoding.EncodeToString([]byte("\x00api\x00senha-do-smtp"))
	if len(srv.auths) != 1 || srv.auths[0] != want || len(srv.mails) != 1 {
		t.Errorf("auths %v, mails %d; want one PLAIN login and one mail", srv.auths, len(srv.mails))
	}
}

// TestSendInvitation_RequireTLSRefusesAServerWithoutStartTLS: with the
// setting on, a reset code or an invitation link is never sent in clear.
func TestSendInvitation_RequireTLSRefusesAServerWithoutStartTLS(t *testing.T) {
	t.Parallel()
	e, token := invitation(t)

	for name, offer := range map[string]bool{"no STARTTLS offered": false, "STARTTLS offered but failing": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv := startServer(t, func(s *server) { s.offerTLS = offer })
			m, err := NewSMTP(SMTPConfig{
				Addr: srv.addr, From: "nao-responda@encomendo.com.br", AdminOrigin: "https://admin.encomendo.com.br",
				Timeout: 2 * time.Second, RequireTLS: true,
			}, slog.New(slog.DiscardHandler))
			if err != nil {
				t.Fatal(err)
			}

			err = m.SendInvitation(t.Context(), e)

			if !errors.Is(err, ErrDelivery) || strings.Contains(err.Error(), token) {
				t.Errorf("SendInvitation = %v; want ErrDelivery", err)
			}
			if n := len(srv.received()); n != 0 {
				t.Errorf("%d mails reached a server without TLS; want none", n)
			}
		})
	}
}

// TestSendInvitation_WithoutRequireTLSSendsToACatcher: a local mail catcher
// has no TLS, and development runs with the setting off.
func TestSendInvitation_WithoutRequireTLSSendsToACatcher(t *testing.T) {
	t.Parallel()
	srv := startServer(t, nil)
	m := newMailer(t, srv.addr, slog.New(slog.DiscardHandler))
	e, _ := invitation(t)

	if err := m.SendInvitation(t.Context(), e); err != nil || len(srv.received()) != 1 {
		t.Errorf("SendInvitation = %v, %d mails; want one mail", err, len(srv.received()))
	}
}
