package notify_test

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/notify"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestWhatsAppLinks_EncodesTheTextWholly(t *testing.T) {
	t.Parallel()
	text := "Olá, João! A entrega mudou de 21/12 para 05/01.\nMotivo: chuva & estrada fechada. 100% certo?"
	link, err := notify.NewWhatsAppLinks().Link("5592981234567", text)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" || u.Host != "wa.me" || u.Path != "/5592981234567" {
		t.Errorf("link = %q", link)
	}
	if got := u.Query().Get("text"); got != text {
		t.Errorf("text = %q, want it back unchanged", got)
	}
	if strings.ContainsAny(u.RawQuery[len("text="):], "&\n ") {
		t.Errorf("raw query %q holds an unescaped &, line break or space", u.RawQuery)
	}
}

func TestWhatsAppLinks_RefusesBadPhonesAndTexts(t *testing.T) {
	t.Parallel()
	links := notify.NewWhatsAppLinks()
	for _, phone := range []string{"", "92981234567", "+5592981234567", "55 92 98123-4567", "5509981234567", "559298123456789"} {
		if _, err := links.Link(phone, "Olá"); !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("phone %q: err = %v", phone, err)
		}
	}
	for name, text := range map[string]string{"empty": "", "too long": strings.Repeat("a", 2001), "invalid utf-8": "\xff"} {
		if _, err := links.Link("5592981234567", text); !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
