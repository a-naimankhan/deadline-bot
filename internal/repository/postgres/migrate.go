package postgres

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"deadline-bot/internal/logger"
)

// Migrate накатывает все новые миграции из fsys. Уже применённые goose пропускает.
// Advisory lock Postgres не даёт двум копиям бота мигрировать одновременно.
func Migrate(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) error {
	// Закрытие этого *sql.DB не закрывает pool.
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("create migration locker: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, fsys, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}

	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	log := logger.Get()
	for _, r := range results {
		log.Info("migration applied", "file", r.Source.Path, "duration", r.Duration.String())
	}
	return nil
}
