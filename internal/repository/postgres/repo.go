package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DB — только то, что нужно репозиторию. *pgxpool.Pool и pgxmock.PgxPoolIface его удовлетворяют.
type DB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Repo struct {
	db DB
}

func New(db DB) *Repo {
	return &Repo{db: db}
}
