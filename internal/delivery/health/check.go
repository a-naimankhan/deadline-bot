package health

import (
	"context"
	"fmt"
	"net/http"
)

// Check делает GET url и возвращает nil только при 200.
// Нужен режиму `bot -healthcheck`: в distroless нет curl/wget, проверяет сам бинарник.
func Check(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build healthcheck request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("healthcheck request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: unexpected status %d", resp.StatusCode)
	}
	return nil
}
