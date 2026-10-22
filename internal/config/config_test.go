package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsDotEnvFromWorkingDirectory(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	for _, key := range []string{"TELEGRAM_BOT_TOKEN", "OWNER_CHAT_ID", "DATABASE_URL"} {
		value, existed := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if existed {
				if err := os.Setenv(key, value); err != nil {
					t.Errorf("restore %s: %v", key, err)
				}
			} else if err := os.Unsetenv(key); err != nil {
				t.Errorf("unset %s: %v", key, err)
			}
		})
	}

	dotenv := "TELEGRAM_BOT_TOKEN=test-token\nOWNER_CHAT_ID=12345\nDATABASE_URL=postgres://localhost/test\n"
	if err := os.WriteFile(filepath.Join(".", ".env"), []byte(dotenv), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.TelegramToken != "test-token" {
		t.Errorf("TelegramToken = %q, want test-token", cfg.TelegramToken)
	}
	if cfg.OwnerChatID != 12345 {
		t.Errorf("OwnerChatID = %d, want 12345", cfg.OwnerChatID)
	}
	if cfg.DatabaseURL != "postgres://localhost/test" {
		t.Errorf("DatabaseURL = %q, want postgres://localhost/test", cfg.DatabaseURL)
	}
}
