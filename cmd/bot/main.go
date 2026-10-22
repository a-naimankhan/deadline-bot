package main

import (
	"context"
	"deadline-bot/internal/config"
	"deadline-bot/internal/logger"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	//TODO run the app
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	defer stop()
	//TODO get crediantials
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

	//TODO init repo

	//TODO init srv

	//TODO init delivery

	<-ctx.Done()
	log.Info("shutting down bot")
	return nil
}
