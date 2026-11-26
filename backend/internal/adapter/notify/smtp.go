package notify

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strings"
	"sync"
	"time"

	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
)

// ErrDelivery means a message could not be handed to the SMTP server. Its
// text names the step that failed and never the recipient, the link or the
// code: the API logs it, and an SMTP server's own reply may quote the
// address (ADR 0011).
var ErrDelivery = errors.New("notify: email not delivered")

// SMTPConfig is where and as whom the mail goes out.
type SMTPConfig struct {
	// Addr is host:port of the SMTP server.
	Addr string
	// From is the sender address; FromName is shown beside it.
	From, FromName string
	// Username and Password sign in when set. Plain auth over a connection
	// the server has not upgraded to TLS is refused by net/smtp, except to
	// localhost.
	Username, Password string
	// AdminOrigin is where invitation links point: https://{admin host},
	// with no trailing slash.
	AdminOrigin string
	// Timeout bounds one delivery, connection and conversation together.
	Timeout time.Duration
	// RequireTLS refuses to send to a server that does not offer STARTTLS.
	// Without it, a server that omits the offer (or a path that strips it)
	// receives the message in clear: a reset code or an invitation link.
	// Production turns it on; only a local mail catcher runs without it.
	RequireTLS bool
}

// SMTP sends the emails of milestone 2 over SMTP, with the standard
// library's net/smtp and no new dependency (ADR 0011). It implements
// appidentity.ResetCodeMailer and apptenancy.InvitationMailer. The text is
// pt-BR plain text.
type SMTP struct {
	cfg  SMTPConfig
	from mail.Address
	log  *slog.Logger
	now  func() time.Time
	wg   sync.WaitGroup
}

var (
	_ appidentity.ResetCodeMailer = (*SMTP)(nil)
	_ apptenancy.InvitationMailer = (*SMTP)(nil)
)

// NewSMTP returns the mailer. It returns an error for a sender address, an
// admin origin or a server address it cannot use.
func NewSMTP(cfg SMTPConfig, log *slog.Logger) (*SMTP, error) {
	from, err := mail.ParseAddress(cfg.From)
	if err != nil {
		return nil, errors.New("notify: the SMTP sender is not an email address")
	}
	if _, _, err := net.SplitHostPort(cfg.Addr); err != nil || cfg.Timeout <= 0 {
		return nil, errors.New("notify: SMTP needs host:port and a timeout")
	}
	if (!strings.HasPrefix(cfg.AdminOrigin, "https://") && !strings.HasPrefix(cfg.AdminOrigin, "http://")) || strings.HasSuffix(cfg.AdminOrigin, "/") {
		return nil, errors.New("notify: the admin origin must be http(s)://host, without a trailing slash")
	}
	from.Name = cfg.FromName
	return &SMTP{cfg: cfg, from: *from, log: log, now: time.Now}, nil
}

// SendResetCode sends the "Esqueci a senha" code and returns at once: the
// delivery runs on its own goroutine with its own timeout, so the answer's
// timing never tells whether the account exists, and a request that ends
// does not stop it. A failure is logged, never returned (ADR 0011).
func (s *SMTP) SendResetCode(ctx context.Context, e appidentity.ResetCodeEmail) {
	body := fmt.Sprintf(`Olá, %s.

Seu código para criar uma nova senha no Encomendo é:

    %s

Ele vale por %d minutos. Se você não pediu uma nova senha, ignore este e-mail: sua senha continua a mesma.
`, e.Name.String(), e.Code.Reveal(), int(identity.ResetCodeLifetime/time.Minute))
	msg := s.compose(e.To.String(), "Seu código para criar uma nova senha", body)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.cfg.Timeout)
		defer cancel()
		if err := s.deliver(ctx, e.To.String(), msg); err != nil {
			s.log.Error("reset code email not sent", "err", err)
		}
	}()
}

// SendInvitation sends the "Convite" email before it returns. It returns an
// ErrDelivery, whose text never carries the address or the link.
func (s *SMTP) SendInvitation(ctx context.Context, e apptenancy.InvitationEmail) error {
	link := s.cfg.AdminOrigin + "/convite#token=" + e.Token.Reveal()
	body := fmt.Sprintf(`Olá!

%s convidou você para a equipe da loja %s no Encomendo, como %s.

Para aceitar, abra este link:

    %s

O convite vale até %s. Se você não esperava este convite, ignore este e-mail.
`, e.InvitedBy, e.Store, roleName(string(e.Role)), link, e.ExpiresAt.In(brazilTime).Format("02/01/2006"))
	msg := s.compose(e.To.String(), "Convite para a equipe de "+e.Store, body)
	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()
	return s.deliver(ctx, e.To.String(), msg)
}

// Wait returns once every reset code in flight has been sent or has failed,
// for a clean shutdown and for tests.
func (s *SMTP) Wait() { s.wg.Wait() }

// brazilTime is where the dates in the emails are read: Brasília time, UTC-3,
// which most stores keep, and Brazil has had no daylight saving since 2019.
var brazilTime = time.FixedZone("BRT", -3*60*60)

func roleName(role string) string {
	switch role {
	case "admin":
		return "administrador"
	case "staff":
		return "funcionário"
	default:
		return "somente leitura"
	}
}

// compose builds the message. The subject is Q-encoded when it holds
// anything but plain ASCII, which also escapes any control character, so a
// store name cannot add a header.
func (s *SMTP) compose(to, subject, body string) []byte {
	var b bytes.Buffer
	var id [12]byte
	_, _ = rand.Read(id[:]) // crypto/rand.Read never fails (Go 1.24+)
	host := s.from.Address[strings.LastIndex(s.from.Address, "@")+1:]
	fmt.Fprintf(&b, "From: %s\r\n", s.from.String())
	fmt.Fprintf(&b, "To: %s\r\n", (&mail.Address{Address: to}).String())
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	fmt.Fprintf(&b, "Date: %s\r\n", s.now().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <%s@%s>\r\n", hex.EncodeToString(id[:]), host)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	w := quotedprintable.NewWriter(&b)
	_, _ = w.Write([]byte(strings.ReplaceAll(body, "\n", "\r\n"))) // a bytes.Buffer never fails
	_ = w.Close()
	return b.Bytes()
}

// deliver hands msg to the server. Every failure is an ErrDelivery naming
// only the step, plus the numeric SMTP reply code when there is one.
func (s *SMTP) deliver(ctx context.Context, to string, msg []byte) error {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", s.cfg.Addr)
	if err != nil {
		return stepError("connect", nil)
	}
	defer func() { _ = conn.Close() }() // the message is sent or lost by now
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline) // an unusable deadline only loses the early cut-off
	host, _, _ := net.SplitHostPort(s.cfg.Addr)
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return stepError("greeting", err)
	}
	defer func() { _ = c.Close() }()
	if err := s.conversation(c, host, to, msg); err != nil {
		return err
	}
	_ = c.Quit() // the server has the message; a failed goodbye changes nothing
	return nil
}

func (s *SMTP) conversation(c *smtp.Client, host, to string, msg []byte) error {
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return stepError("starttls", err)
		}
	} else if s.cfg.RequireTLS {
		return stepError("starttls", nil)
	}
	if s.cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, host)); err != nil {
			return stepError("auth", err)
		}
	}
	if err := c.Mail(s.from.Address); err != nil {
		return stepError("sender", err)
	}
	if err := c.Rcpt(to); err != nil {
		return stepError("recipient", err)
	}
	w, err := c.Data()
	if err != nil {
		return stepError("data", err)
	}
	if _, err := w.Write(msg); err != nil {
		return stepError("data", err)
	}
	if err := w.Close(); err != nil {
		return stepError("data", err)
	}
	return nil
}

// stepError names the step and, for an SMTP reply, its numeric code. The
// reply text stays out: servers quote the recipient in it.
func stepError(step string, err error) error {
	var tp *textproto.Error
	if errors.As(err, &tp) {
		return fmt.Errorf("smtp %s (reply %d): %w", step, tp.Code, ErrDelivery)
	}
	return fmt.Errorf("smtp %s: %w", step, ErrDelivery)
}
