package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	TelegramToken string
	OwnerChatID   int64
	DatabaseURL   string
	Location      *time.Location
	ReminderHour  int
	Tick          time.Duration
	Workers       int
	LogLevel      string
	HTTPPort      int
}

const (
	defaultTZ       = "Asia/Almaty"
	defaultReminder = 12
	defaultTick     = time.Minute
	defaultWorkers  = 3
	defaultLogLevel = "info"
	defaultPort     = 8080
)

func Load() (*Config, error) {
	var missing []string
	required := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}

	token := required("TELEGRAM_BOT_TOKEN")
	rawChatID := required("OWNER_CHAT_ID")
	dsn := required("DATABASE_URL")
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required env: %s", strings.Join(missing, ", "))
	}

	chatID, err := strconv.ParseInt(rawChatID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse OWNER_CHAT_ID: %w", err)
	}

	port, err := LoadPort()
	if err != nil {
		return nil, err
	}

	loc, err := time.LoadLocation(defaultTZ)
	if err != nil {
		return nil, fmt.Errorf("load location %q: %w", defaultTZ, err)
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = defaultLogLevel
	}

	return &Config{
		TelegramToken: token,
		OwnerChatID:   chatID,
		DatabaseURL:   dsn,
		Location:      loc,
		ReminderHour:  defaultReminder,
		Tick:          defaultTick,
		Workers:       defaultWorkers,
		LogLevel:      logLevel,
		HTTPPort:      port,
	}, nil
}

// LoadPort читает необязательный PORT (по умолчанию 8080, допустимо 1..65535).
// Отдельно от Load: режиму -healthcheck не нужны токен и DSN.
func LoadPort() (int, error) {
	raw := os.Getenv("PORT")
	if raw == "" {
		return defaultPort, nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parse PORT: %w", err)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("parse PORT: %d is out of range 1..65535", port)
	}
	return port, nil
}
