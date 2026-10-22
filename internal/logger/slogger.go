package logger

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
)

var (
	once    sync.Once
	initErr error
)

func Init(level string) error {
	once.Do(func() {
		var lvl slog.Level
		if err := lvl.UnmarshalText([]byte(level)); err != nil {
			initErr = fmt.Errorf("parse log level %q: %w", level, err)
			return
		}

		handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level:     lvl,
			AddSource: lvl <= slog.LevelDebug,
		})
		slog.SetDefault(slog.New(handler))
	})
	return initErr
}

func Get() *slog.Logger {
	return slog.Default()
}
