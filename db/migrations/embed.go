// Package migrations embeds the SQL migration files so they can be applied
// from the application binary via golang-migrate's iofs source driver.
package migrations

import "embed"

// FS holds the embedded *.up.sql / *.down.sql migration files.
//
//go:embed *.sql
var FS embed.FS
