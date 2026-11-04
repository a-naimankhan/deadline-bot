// Package health — HTTP-эндпоинт /healthz для Docker HEALTHCHECK и хостинга.
package health

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"deadline-bot/internal/logger"
)

const (
	// PingTimeout — сколько ждём ответа БД, прежде чем считать бота нездоровым.
	PingTimeout     = 2 * time.Second
	shutdownTimeout = 5 * time.Second
)

// Pinger — то, что health-слою нужно от БД. *pgxpool.Pool его реализует.
type Pinger interface {
	Ping(ctx context.Context) error
}

// NewHandler отдаёт GET /healthz: 200 "ok", если БД ответила за pingTimeout, иначе 503.
// Причина ошибки пишется только в лог: в теле ответа ничего внутреннего.
// Другие методы получают 405 от ServeMux (паттерн "GET ..." пропускает и HEAD).
func NewHandler(db Pinger, pingTimeout time.Duration) http.Handler {
	log := logger.Get()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), pingTimeout)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			log.Warn("healthz: db ping failed", "err", err)
			http.Error(w, "db unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "ok")
	})
	return mux
}

type Server struct {
	srv *http.Server
	log *slog.Logger
}

// NewServer создаёт HTTP-сервер на addr (например ":8080"). Слушать начинает Run.
func NewServer(addr string, db Pinger) *Server {
	return &Server{
		srv: &http.Server{
			Addr:              addr,
			Handler:           NewHandler(db, PingTimeout),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       5 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		log: logger.Get(),
	}
}

// Run слушает до отмены ctx, затем мягко останавливает сервер (Shutdown с таймаутом).
// Ошибка bind (порт занят) возвращается сразу — errgroup остановит остальной бот.
func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.srv.Addr)
	if err != nil {
		return fmt.Errorf("health server listen %s: %w", s.srv.Addr, err)
	}
	s.log.Info("health server listening", "addr", ln.Addr().String())

	errCh := make(chan error, 1)
	go func() { errCh <- s.srv.Serve(ln) }()

	select {
	case err := <-errCh:
		return fmt.Errorf("health server: %w", err)
	case <-ctx.Done():
	}

	// ctx уже отменён — для Shutdown нужен свой контекст с таймаутом.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := s.srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown health server: %w", err)
	}
	if err := <-errCh; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("health server: %w", err)
	}
	return nil
}
