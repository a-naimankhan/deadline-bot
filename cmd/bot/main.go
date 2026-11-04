package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
	_ "time/tzdata" // Asia/Almaty работает и в минимальном Docker-образе без системной tzdata

	"github.com/joho/godotenv"
	"golang.org/x/sync/errgroup"

	"deadline-bot/internal/config"
	"deadline-bot/internal/delivery/telegram"
	"deadline-bot/internal/logger"
	"deadline-bot/internal/repository/postgres"
	"deadline-bot/internal/scheduler"
	"deadline-bot/internal/service"
	"deadline-bot/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func loadDotEnv() error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	for {
		envPath := filepath.Join(dir, ".env")
		if _, err := os.Stat(envPath); err == nil {
			if err := godotenv.Load(envPath); err != nil {
				return fmt.Errorf("parse %s: %w", envPath, err)
			}
			return nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return nil
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := loadDotEnv(); err != nil {
		return fmt.Errorf("load .env: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := logger.Init(cfg.LogLevel); err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	log := logger.Get()
	log.Info("starting bot", "tz", cfg.Location.String(), "workers", cfg.Workers)

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("init pool: %w", err)
	}
	defer pool.Close()

	if err := postgres.Migrate(ctx, pool, migrations.FS); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	repo := postgres.New(pool)
	svc := service.New(repo, cfg.Location, cfg.ReminderHour, time.Now)

	bot, err := telegram.New(cfg.TelegramToken, cfg.OwnerChatID, svc, cfg.Location, time.Now)
	if err != nil {
		return fmt.Errorf("init bot: %w", err)
	}
	sched := scheduler.NewScheduler(repo, bot.Notifier(), cfg.Workers, cfg.Tick, time.Now)

	// Если одна часть упала с ошибкой, gctx отменяется и вторая тоже останавливается.
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return bot.Run(gctx) })
	g.Go(func() error { return sched.Run(gctx) })

	log.Info("bot is running")
	err = g.Wait()
	log.Info("shutting down bot")
	if err != nil {
		return fmt.Errorf("run: %w", err)
	}
	return nil
}
