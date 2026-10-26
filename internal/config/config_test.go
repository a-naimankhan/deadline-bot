package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name        string
		setEnv      func()
		wantErr     bool
		errContains string
		checkConfig func(*testing.T, *Config)
	}{
		{
			name: "all required set with defaults",
			setEnv: func() {
				os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
				os.Setenv("OWNER_CHAT_ID", "12345")
				os.Setenv("DATABASE_URL", "postgres://localhost/test")
			},
			wantErr: false,
			checkConfig: func(t *testing.T, cfg *Config) {
				if cfg.TelegramToken != "test-token" {
					t.Errorf("TelegramToken mismatch")
				}
				if cfg.OwnerChatID != 12345 {
					t.Errorf("OwnerChatID = %d, want 12345", cfg.OwnerChatID)
				}
				if cfg.DatabaseURL != "postgres://localhost/test" {
					t.Errorf("DatabaseURL mismatch")
				}
				if cfg.LogLevel != "info" {
					t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
				}
				if cfg.Workers != 3 {
					t.Errorf("Workers = %d, want 3", cfg.Workers)
				}
				if cfg.ReminderHour != 12 {
					t.Errorf("ReminderHour = %d, want 12", cfg.ReminderHour)
				}
				if cfg.Tick != time.Minute {
					t.Errorf("Tick = %v, want 1m", cfg.Tick)
				}
				if cfg.Location.String() != "Asia/Almaty" {
					t.Errorf("Location = %s, want Asia/Almaty", cfg.Location)
				}
			},
		},
		{
			name: "missing TELEGRAM_BOT_TOKEN",
			setEnv: func() {
				os.Setenv("OWNER_CHAT_ID", "12345")
				os.Setenv("DATABASE_URL", "postgres://localhost/test")
			},
			wantErr:     true,
			errContains: "TELEGRAM_BOT_TOKEN",
		},
		{
			name: "missing DATABASE_URL",
			setEnv: func() {
				os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
				os.Setenv("OWNER_CHAT_ID", "12345")
			},
			wantErr:     true,
			errContains: "DATABASE_URL",
		},
		{
			name: "missing OWNER_CHAT_ID",
			setEnv: func() {
				os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
				os.Setenv("DATABASE_URL", "postgres://localhost/test")
			},
			wantErr:     true,
			errContains: "OWNER_CHAT_ID",
		},
		{
			name: "OWNER_CHAT_ID not a number",
			setEnv: func() {
				os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
				os.Setenv("OWNER_CHAT_ID", "not-a-number")
				os.Setenv("DATABASE_URL", "postgres://localhost/test")
			},
			wantErr:     true,
			errContains: "parse OWNER_CHAT_ID",
		},
		{
			name: "multiple missing variables",
			setEnv: func() {
				os.Setenv("OWNER_CHAT_ID", "12345")
			},
			wantErr:     true,
			errContains: "TELEGRAM_BOT_TOKEN",
		},
		{
			name: "LOG_LEVEL set explicitly",
			setEnv: func() {
				os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
				os.Setenv("OWNER_CHAT_ID", "12345")
				os.Setenv("DATABASE_URL", "postgres://localhost/test")
				os.Setenv("LOG_LEVEL", "debug")
			},
			wantErr: false,
			checkConfig: func(t *testing.T, cfg *Config) {
				if cfg.LogLevel != "debug" {
					t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := []string{"TELEGRAM_BOT_TOKEN", "OWNER_CHAT_ID", "DATABASE_URL", "LOG_LEVEL"}
			originalValues := make(map[string]string)
			originalExisted := make(map[string]bool)

			for _, key := range keys {
				val, existed := os.LookupEnv(key)
				originalValues[key] = val
				originalExisted[key] = existed
				os.Unsetenv(key)
			}

			t.Cleanup(func() {
				for _, key := range keys {
					if originalExisted[key] {
						os.Setenv(key, originalValues[key])
					} else {
						os.Unsetenv(key)
					}
				}
			})

			tt.setEnv()

			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Load() expected error, got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Load() error = %v, should contain %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			if tt.checkConfig != nil {
				tt.checkConfig(t, cfg)
			}
		})
	}
}
