package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNewPoolInvalidDSN(t *testing.T) {
	ctx := context.Background()
	_, err := NewPool(ctx, "invalid://dsn")
	if err == nil {
		t.Error("expected error for invalid DSN")
	}
}

func TestNewPoolValidDSN(t *testing.T) {
	ctx := context.Background()
	pool, err := NewPool(ctx, "postgres://localhost:5432/test?sslmode=disable")
	if err != nil {
		t.Skip("postgres not available, skipping connection test")
	}
	defer pool.Close()
	if pool == nil {
		t.Error("expected non-nil pool")
	}
}

func TestNewPoolNilContext(t *testing.T) {
	var pool *pgxpool.Pool
	var err error
	defer func() {
		if r := recover(); r != nil {
			t.Logf("panic with nil context: %v", r)
		}
	}()
	pool, err = NewPool(nil, "postgres://localhost:5432/test")
	if err == nil && pool != nil {
		t.Error("expected error with nil context")
	}
}
