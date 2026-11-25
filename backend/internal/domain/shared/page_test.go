package shared

import (
	"errors"
	"strings"
	"testing"
)

func TestNewPageRequest_DefaultsAndBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cursor    string
		limit     int
		wantLimit int
		wantField string
	}{
		{name: "no limit takes the default", limit: 0, wantLimit: DefaultPageLimit},
		{name: "one", limit: 1, wantLimit: 1},
		{name: "the maximum", limit: MaxPageLimit, wantLimit: MaxPageLimit},
		{name: "keeps the cursor", cursor: "abc", limit: 5, wantLimit: 5},
		{name: "above the maximum", limit: MaxPageLimit + 1, wantField: "limit"},
		{name: "negative", limit: -1, wantField: "limit"},
		{name: "cursor too long", cursor: strings.Repeat("a", MaxCursorLength+1), wantField: "cursor"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, err := NewPageRequest(tt.cursor, tt.limit)
			if tt.wantField != "" {
				var ve *ValidationError
				if !errors.As(err, &ve) || len(ve.Fields) != 1 || ve.Fields[0].Field != tt.wantField {
					t.Fatalf("error = %v, want a ValidationError on %q", err, tt.wantField)
				}
				if ve.Fields[0].Message == "" {
					t.Error("the field error has no pt-BR message")
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if p.Limit() != tt.wantLimit || p.Cursor() != tt.cursor {
				t.Errorf("got limit %d cursor %q", p.Limit(), p.Cursor())
			}
		})
	}
}

func TestPage_HasMore(t *testing.T) {
	t.Parallel()

	if (Page[int]{Items: []int{1}}).HasMore() {
		t.Error("a page without a next cursor reports HasMore")
	}
	if !(Page[int]{Items: []int{1}, NextCursor: "x"}).HasMore() {
		t.Error("a page with a next cursor reports !HasMore")
	}
}
