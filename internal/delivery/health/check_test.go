package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCheck(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		ctxTimeout  time.Duration
		wantErr     bool
		errContains string
		maxElapsed  time.Duration
	}{
		{
			name: "200 ok",
			setup: func(t *testing.T) string {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = w.Write([]byte("ok"))
				}))
				t.Cleanup(srv.Close)
				return srv.URL + "/healthz"
			},
			wantErr: false,
		},
		{
			name: "503 is error",
			setup: func(t *testing.T) string {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "db unavailable", http.StatusServiceUnavailable)
				}))
				t.Cleanup(srv.Close)
				return srv.URL + "/healthz"
			},
			wantErr:     true,
			errContains: "503",
		},
		{
			name: "500 is error",
			setup: func(t *testing.T) string {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
				t.Cleanup(srv.Close)
				return srv.URL + "/healthz"
			},
			wantErr:     true,
			errContains: "500",
		},
		{
			name: "connection refused",
			setup: func(t *testing.T) string {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = w.Write([]byte("ok"))
				}))
				url := srv.URL
				srv.Close()
				return url + "/healthz"
			},
			wantErr: true,
		},
		{
			name: "timeout",
			setup: func(t *testing.T) string {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					select {
					case <-r.Context().Done():
					case <-time.After(2 * time.Second):
					}
				}))
				t.Cleanup(srv.Close)
				return srv.URL + "/healthz"
			},
			ctxTimeout: 100 * time.Millisecond,
			wantErr:    true,
			maxElapsed: time.Second,
		},
		{
			name: "invalid url",
			setup: func(t *testing.T) string {
				return "://bad"
			},
			wantErr:     true,
			errContains: "build healthcheck request",
		},
		{
			name: "real handler, db ok",
			setup: func(t *testing.T) string {
				srv := httptest.NewServer(NewHandler(&fakePinger{}, time.Second))
				t.Cleanup(srv.Close)
				return srv.URL + "/healthz"
			},
			wantErr: false,
		},
		{
			name: "real handler, db down",
			setup: func(t *testing.T) string {
				srv := httptest.NewServer(NewHandler(&fakePinger{err: errors.New("down")}, time.Second))
				t.Cleanup(srv.Close)
				return srv.URL + "/healthz"
			},
			wantErr:     true,
			errContains: "503",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := tt.setup(t)

			timeout := tt.ctxTimeout
			if timeout == 0 {
				timeout = 2 * time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			start := time.Now()
			err := Check(ctx, url)
			elapsed := time.Since(start)

			if tt.wantErr {
				if err == nil {
					t.Fatal("Check() expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Check() error = %v, should contain %q", err, tt.errContains)
				}
			} else if err != nil {
				t.Fatalf("Check() unexpected error: %v", err)
			}

			if tt.maxElapsed > 0 && elapsed >= tt.maxElapsed {
				t.Errorf("Check() took %v, want < %v", elapsed, tt.maxElapsed)
			}
		})
	}
}
