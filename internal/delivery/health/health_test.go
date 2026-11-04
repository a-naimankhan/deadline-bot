package health

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakePinger — подменяет БД: отвечает err, либо (block) ждёт отмены ctx.
type fakePinger struct {
	err   error
	block bool
	calls int
}

func (f *fakePinger) Ping(ctx context.Context) error {
	f.calls++
	if f.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.err
}

func TestHandler(t *testing.T) {
	tests := []struct {
		name        string
		pinger      *fakePinger
		method      string
		path        string
		wantCode    int
		checkBody   bool
		wantBody    string
		notInBody   string
		checkCT     bool
		wantAllow   string
		wantCalls   int
		maxDuration time.Duration
	}{
		{
			name:      "db ok",
			pinger:    &fakePinger{},
			method:    http.MethodGet,
			path:      "/healthz",
			wantCode:  http.StatusOK,
			checkBody: true,
			wantBody:  "ok",
			checkCT:   true,
			wantCalls: 1,
		},
		{
			name:      "db error",
			pinger:    &fakePinger{err: errors.New("dial tcp: password=hunter2")},
			method:    http.MethodGet,
			path:      "/healthz",
			wantCode:  http.StatusServiceUnavailable,
			checkBody: true,
			wantBody:  "db unavailable",
			notInBody: "hunter2",
			wantCalls: 1,
		},
		{
			name:        "db ping exceeds timeout",
			pinger:      &fakePinger{block: true},
			method:      http.MethodGet,
			path:        "/healthz",
			wantCode:    http.StatusServiceUnavailable,
			checkBody:   true,
			wantBody:    "db unavailable",
			wantCalls:   1,
			maxDuration: time.Second,
		},
		{
			// httptest.ResponseRecorder не отбрасывает тело для HEAD (как делает настоящий сервер),
			// поэтому тело здесь не проверяем — важен код ответа и факт разрешения метода.
			name:      "head is allowed",
			pinger:    &fakePinger{},
			method:    http.MethodHead,
			path:      "/healthz",
			wantCode:  http.StatusOK,
			wantCalls: 1,
		},
		{
			name:      "post not allowed",
			pinger:    &fakePinger{},
			method:    http.MethodPost,
			path:      "/healthz",
			wantCode:  http.StatusMethodNotAllowed,
			wantAllow: "GET",
			wantCalls: 0,
		},
		{
			name:      "unknown path",
			pinger:    &fakePinger{},
			method:    http.MethodGet,
			path:      "/foo",
			wantCode:  http.StatusNotFound,
			wantCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(tt.pinger, 50*time.Millisecond)
			rec := httptest.NewRecorder()

			start := time.Now()
			h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			elapsed := time.Since(start)

			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if tt.checkBody {
				if got := strings.TrimSpace(rec.Body.String()); got != tt.wantBody {
					t.Errorf("body = %q, want %q", got, tt.wantBody)
				}
			}
			if tt.notInBody != "" && strings.Contains(rec.Body.String(), tt.notInBody) {
				t.Errorf("body %q must not contain %q", rec.Body.String(), tt.notInBody)
			}
			if tt.checkCT {
				if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
					t.Errorf("Content-Type = %q, want prefix text/plain", ct)
				}
			}
			if tt.wantAllow != "" && !strings.Contains(rec.Header().Get("Allow"), tt.wantAllow) {
				t.Errorf("Allow = %q, want it to contain %q", rec.Header().Get("Allow"), tt.wantAllow)
			}
			if tt.maxDuration > 0 && elapsed >= tt.maxDuration {
				t.Errorf("ServeHTTP took %v, want < %v", elapsed, tt.maxDuration)
			}
			if tt.pinger.calls != tt.wantCalls {
				t.Errorf("Ping calls = %d, want %d", tt.pinger.calls, tt.wantCalls)
			}
		})
	}
}

func TestServerRun(t *testing.T) {
	t.Run("stops on context cancel", func(t *testing.T) {
		s := NewServer("127.0.0.1:0", &fakePinger{})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		errCh := make(chan error, 1)
		go func() { errCh <- s.Run(ctx) }()

		time.Sleep(50 * time.Millisecond)
		cancel()

		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("Run() returned error after cancel: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("Run() did not return after context cancel")
		}
	})

	t.Run("cancelled before start", func(t *testing.T) {
		s := NewServer("127.0.0.1:0", &fakePinger{})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		errCh := make(chan error, 1)
		go func() { errCh <- s.Run(ctx) }()

		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("Run() returned error for cancelled ctx: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("Run() did not return for already cancelled ctx")
		}
	})

	t.Run("port already in use", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()

		s := NewServer(ln.Addr().String(), &fakePinger{})

		errCh := make(chan error, 1)
		go func() { errCh <- s.Run(context.Background()) }()

		select {
		case err := <-errCh:
			if err == nil {
				t.Fatal("Run() returned nil, want bind error")
			}
			if !strings.Contains(err.Error(), "listen") {
				t.Errorf("Run() error = %v, should contain %q", err, "listen")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Run() did not return bind error in time")
		}
	})
}
