// Package migrations вшивает SQL-миграции goose в бинарник, чтобы бот сам накатывал их при старте.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
