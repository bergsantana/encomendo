// Package migrations embeds the SQL migrations into the binary.
//
// Files are goose SQL migrations named NNNN_what.sql, each with an Up and a
// Down section. They run as encomendo_owner through cmd/migrate, never as the
// API's encomendo_app role. Use the new-migration skill to add one.
package migrations

import "embed"

// FS holds every migration file.
//
//go:embed *.sql
var FS embed.FS
