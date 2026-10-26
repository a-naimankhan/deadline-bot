package main

import (
	"context"
	"deadline-bot/internal/config"
	"deadline-bot/internal/logger"
	"deadline-bot/internal/repository/postgres"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/joho/godotenv"
)

func main() {
	//TODO run the app
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

	//TODO init logger
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

	//TODO init repo
	repo := postgres.New(pool)
	_ = repo //временно

	//TODO init srv

	//TODO init delivery

	<-ctx.Done()
	log.Info("shutting down bot")
	return nil
}
