package postgres

import (
	"context"
	"testing"
)

func TestNewPoolInvalidDSN(t *testing.T) {
	ctx := context.Background()
	_, err := NewPool(ctx, "invalid://dsn")
	if err == nil {
		t.Error("expected error for invalid DSN")
	}
}
