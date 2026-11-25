package migrations

import (
	"io/fs"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var fileName = regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.sql$`)

// TestMigrationVersions_AreContiguous enforces ADR 0014: a migration takes
// the next free number, so the versions are exactly 1, 2, 3 … with no gap
// and no duplicate. goose refuses a file numbered below the database's
// current version, and CI, which migrates an empty database, would not see it.
func TestMigrationVersions_AreContiguous(t *testing.T) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	want := 1
	for _, e := range entries { // ReadDir sorts by name, so by version
		m := fileName.FindStringSubmatch(e.Name())
		if m == nil {
			t.Errorf("%s: want NNNN_what.sql, lower case", e.Name())
			continue
		}
		got, _ := strconv.Atoi(m[1]) // four digits, always a number
		if got != want {
			t.Fatalf("%s has version %d, want %d: take the next free number (ADR 0014)", e.Name(), got, want)
		}
		want++
	}
	if want == 1 {
		t.Fatal("no migrations embedded")
	}
}

// TestMigrations_HaveUpAndDown: CI runs up, down and up again, so every file
// needs both sections, Up first.
func TestMigrations_HaveUpAndDown(t *testing.T) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := fs.ReadFile(FS, e.Name())
		if err != nil {
			t.Fatal(err)
		}
		up, down := strings.Index(string(b), "-- +goose Up"), strings.Index(string(b), "-- +goose Down")
		if up < 0 || down < 0 || down < up {
			t.Errorf("%s: want a -- +goose Up section followed by a -- +goose Down section", e.Name())
		}
	}
}
