package checker

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/korotkovfedor/pingwisp/internal/models"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func assertResult(t *testing.T, result models.CheckResult, status string, code int, wantError bool) {
	t.Helper()
	if result.Status != status {
		t.Errorf("status = %q; want %q", result.Status, status)
	}
	if code == 0 {
		if result.StatusCode != nil {
			t.Errorf("status code = %d; want nil", *result.StatusCode)
		}
	} else if result.StatusCode == nil || *result.StatusCode != code {
		t.Errorf("status code = %v; want %d", result.StatusCode, code)
	}
	if (result.Error != nil) != wantError {
		t.Errorf("error = %v; want error: %v", result.Error, wantError)
	}
	if result.CompletedAt.IsZero() || result.Latency < 0 {
		t.Errorf("invalid completion time or latency: %+v", result)
	}
}

func TestHTTPCheckerStatus(t *testing.T) {
	for _, tc := range []struct {
		code   int
		status string
	}{
		{200, models.StatusUp},
		{204, models.StatusUp},
		{299, models.StatusUp},
		{300, models.StatusDown},
		{503, models.StatusDown},
	} {
		t.Run(strconv.Itoa(tc.code), func(t *testing.T) {
			t.Parallel()
			server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/health" {
					t.Errorf("request = %s %s; want GET /health", r.Method, r.URL.Path)
				}
				w.WriteHeader(tc.code)
			}))
			result := NewHTTP(server.Client()).Check(t.Context(), models.Target{URL: "http://example.com/health"})
			assertResult(t, result, tc.status, tc.code, false)
		})
	}
}

func TestHTTPCheckerNetworkError(t *testing.T) {
	t.Parallel()
	networkError := errors.New("connection refused")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, networkError
	})}
	result := NewHTTP(client).Check(t.Context(), models.Target{URL: "http://example.com/health"})
	assertResult(t, result, models.StatusDown, 0, true)
	if !errors.Is(result.Error, networkError) {
		t.Errorf("error = %v; want wrapped network error", result.Error)
	}
}

func TestHTTPCheckerTimeoutWhileReading(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "1")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		}))
		start := time.Now()
		result := NewHTTP(server.Client()).Check(t.Context(), models.Target{URL: "http://example.com/health"})
		assertResult(t, result, models.StatusDown, http.StatusOK, true)
		if !errors.Is(result.Error, context.DeadlineExceeded) {
			t.Errorf("error = %v; want DeadlineExceeded", result.Error)
		}
		if elapsed := time.Since(start); elapsed != 3*time.Second || result.Latency != elapsed {
			t.Errorf("elapsed = %s, latency = %s; want 3s", elapsed, result.Latency)
		}
	})
}

func TestHTTPCheckerCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(started)
			<-r.Context().Done()
		}))
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		results := make(chan models.CheckResult, 1)
		go func() {
			results <- NewHTTP(server.Client()).Check(ctx, models.Target{URL: "http://example.com/health"})
		}()
		<-started
		cancel()
		result := <-results
		assertResult(t, result, models.StatusDown, 0, true)
		if !errors.Is(result.Error, context.Canceled) {
			t.Errorf("error = %v; want Canceled", result.Error)
		}
	})
}

func TestHTTPCheckerReadErrorPreservesCode(t *testing.T) {
	t.Parallel()
	server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "short")
	}))
	result := NewHTTP(server.Client()).Check(t.Context(), models.Target{URL: "http://example.com/health"})
	assertResult(t, result, models.StatusDown, http.StatusServiceUnavailable, true)
	if !errors.Is(result.Error, io.ErrUnexpectedEOF) {
		t.Errorf("error = %v; want UnexpectedEOF", result.Error)
	}
}

func TestHTTPCheckerBodyLimit(t *testing.T) {
	const limit = 1024 * 1024
	for _, size := range []int{limit, limit + 1} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			t.Parallel()
			body := strings.Repeat("x", size)
			server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, body)
			}))
			result := NewHTTP(server.Client()).Check(t.Context(), models.Target{URL: "http://example.com/health"})
			status := models.StatusUp
			if size > limit {
				status = models.StatusDown
			}
			assertResult(t, result, status, http.StatusOK, size > limit)
		})
	}
}

func TestHTTPCheckerRedirects(t *testing.T) {
	t.Run("follows redirects to final response", func(t *testing.T) {
		paths := make(chan string, 3)
		server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			paths <- r.URL.Path
			switch r.URL.Path {
			case "/start":
				http.Redirect(w, r, "/middle", http.StatusFound)
			case "/middle":
				http.Redirect(w, r, "/final", http.StatusTemporaryRedirect)
			case "/final":
				w.WriteHeader(http.StatusNoContent)
			default:
				t.Errorf("unexpected redirect path: %s", r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		result := NewHTTP(server.Client()).Check(t.Context(), models.Target{URL: "http://example.com/start"})
		assertResult(t, result, models.StatusUp, http.StatusNoContent, false)
		for _, want := range []string{"/start", "/middle", "/final"} {
			select {
			case got := <-paths:
				if got != want {
					t.Errorf("redirect path = %q; want %q", got, want)
				}
			default:
				t.Fatalf("missing request to %s", want)
			}
		}
	})

	t.Run("redirect policy error preserves response code", func(t *testing.T) {
		server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/final", http.StatusTemporaryRedirect)
		}))
		redirectError := errors.New("redirect rejected")
		client := server.Client()
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return redirectError }
		result := NewHTTP(client).Check(t.Context(), models.Target{URL: "http://example.com/start"})
		assertResult(t, result, models.StatusDown, http.StatusTemporaryRedirect, true)
		if !errors.Is(result.Error, redirectError) {
			t.Errorf("error = %v; want redirect policy error", result.Error)
		}
	})

	t.Run("default redirect limit", func(t *testing.T) {
		server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/loop", http.StatusFound)
		}))
		result := NewHTTP(server.Client()).Check(t.Context(), models.Target{URL: "http://example.com/loop"})
		assertResult(t, result, models.StatusDown, http.StatusFound, true)
	})
}

func TestHTTPCheckerInvalidURL(t *testing.T) {
	result := NewHTTP(nil).Check(t.Context(), models.Target{URL: "://invalid"})
	assertResult(t, result, models.StatusDown, 0, true)
}
