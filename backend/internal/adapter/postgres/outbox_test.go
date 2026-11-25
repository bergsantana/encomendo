package postgres

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
)

// The stored-and-committed half of the contract runs against Postgres in
// test/integration/postgres; these cases are refused before any SQL.

type testEvent struct {
	Name string `json:"-"`
	Note string `json:"note"`
}

func (e testEvent) EventName() string { return e.Name }

type pointerEvent struct{}

func (*pointerEvent) EventName() string { return "test.pointer" }

type unencodable struct {
	Fn func() `json:"fn"`
}

func (unencodable) EventName() string { return "test.unencodable" }

func TestOutbox_RefusesBeforeAnySQL(t *testing.T) {
	userTx := context.WithValue(context.Background(), txKey{}, &txConn{scope: appshared.Scope{UserID: unitUser}})
	storeTx := context.WithValue(context.Background(), txKey{}, &txConn{scope: appshared.Scope{TenantID: unitTenant}})
	var nilPointer *pointerEvent
	cases := map[string]struct {
		ctx  context.Context
		ev   appshared.Event
		want error
	}{
		"no transaction":    {context.Background(), testEvent{Name: "test.ok"}, appshared.ErrNoTransaction},
		"no tenant":         {userTx, testEvent{Name: "test.ok"}, appshared.ErrEventWithoutTenant},
		"nil":               {storeTx, nil, appshared.ErrInvalidEvent},
		"typed nil pointer": {storeTx, nilPointer, appshared.ErrInvalidEvent},
		"no name":           {storeTx, testEvent{}, appshared.ErrInvalidEvent},
		"not JSON":          {storeTx, unencodable{Fn: func() {}}, appshared.ErrInvalidEvent},
	}
	for name, c := range cases {
		if err := NewOutbox().Publish(c.ctx, c.ev); !errors.Is(err, c.want) {
			t.Errorf("%s: Publish = %v, want %v", name, err, c.want)
		}
	}
}

func TestEncodeEvent_GivesNameAndJSON(t *testing.T) {
	name, payload, err := encodeEvent(testEvent{Name: "test.ok", Note: "Banco de couro"})
	if err != nil || name != "test.ok" || string(payload) != `{"note":"Banco de couro"}` {
		t.Errorf("encodeEvent = %q, %s, %v", name, payload, err)
	}
}
