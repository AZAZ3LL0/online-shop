// Package migrations embeds the goose migration files so that `app migrate`
// carries the schema inside the binary and needs no files on the server.
package migrations

import "embed"

// FS holds every migration, applied in filename order.
//
//go:embed *.sql
var FS embed.FS
