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
				if cfg.HTTPPort != 8080 {
					t.Errorf("HTTPPort = %d, want 8080", cfg.HTTPPort)
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
		{
			name: "PORT set explicitly",
			setEnv: func() {
				os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
				os.Setenv("OWNER_CHAT_ID", "12345")
				os.Setenv("DATABASE_URL", "postgres://localhost/test")
				os.Setenv("PORT", "9090")
			},
			wantErr: false,
			checkConfig: func(t *testing.T, cfg *Config) {
				if cfg.HTTPPort != 9090 {
					t.Errorf("HTTPPort = %d, want 9090", cfg.HTTPPort)
				}
			},
		},
		{
			name: "PORT not a number",
			setEnv: func() {
				os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
				os.Setenv("OWNER_CHAT_ID", "12345")
				os.Setenv("DATABASE_URL", "postgres://localhost/test")
				os.Setenv("PORT", "abc")
			},
			wantErr:     true,
			errContains: "parse PORT",
		},
		{
			name: "PORT zero",
			setEnv: func() {
				os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
				os.Setenv("OWNER_CHAT_ID", "12345")
				os.Setenv("DATABASE_URL", "postgres://localhost/test")
				os.Setenv("PORT", "0")
			},
			wantErr:     true,
			errContains: "parse PORT",
		},
		{
			name: "PORT too large",
			setEnv: func() {
				os.Setenv("TELEGRAM_BOT_TOKEN", "test-token")
				os.Setenv("OWNER_CHAT_ID", "12345")
				os.Setenv("DATABASE_URL", "postgres://localhost/test")
				os.Setenv("PORT", "70000")
			},
			wantErr:     true,
			errContains: "parse PORT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := []string{"TELEGRAM_BOT_TOKEN", "OWNER_CHAT_ID", "DATABASE_URL", "LOG_LEVEL", "PORT"}
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

// TestLoadPort проверяет LoadPort отдельно от Load: режим healthcheck не должен требовать секреты.
func TestLoadPort(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		set     bool
		want    int
		wantErr bool
	}{
		{name: "unset gives default", set: false, want: 8080},
		{name: "empty gives default", value: "", set: true, want: 8080},
		{name: "valid", value: "8081", set: true, want: 8081},
		{name: "min boundary", value: "1", set: true, want: 1},
		{name: "max boundary", value: "65535", set: true, want: 65535},
		{name: "not a number", value: "abc", set: true, wantErr: true},
		{name: "zero", value: "0", set: true, wantErr: true},
		{name: "negative", value: "-1", set: true, wantErr: true},
		{name: "just above max", value: "65536", set: true, wantErr: true},
		{name: "way too large", value: "70000", set: true, wantErr: true},
		{name: "with spaces", value: " 8080", set: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Секреты пустые: LoadPort не должен от них зависеть.
			t.Setenv("TELEGRAM_BOT_TOKEN", "")
			t.Setenv("OWNER_CHAT_ID", "")
			t.Setenv("DATABASE_URL", "")
			if tt.set {
				t.Setenv("PORT", tt.value)
			} else {
				t.Setenv("PORT", "")
				os.Unsetenv("PORT")
			}

			got, err := LoadPort()
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "PORT") {
					t.Errorf("LoadPort() error = %v, want error containing PORT", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadPort() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("LoadPort() = %d, want %d", got, tt.want)
			}
		})
	}
}
