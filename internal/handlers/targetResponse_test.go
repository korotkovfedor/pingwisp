package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/korotkovfedor/pingwisp/internal/models"
	"github.com/korotkovfedor/pingwisp/internal/poller"
)

func TestTargetResponseJSON(t *testing.T) {
	completedAt := time.Date(2026, 9, 29, 9, 10, 32, 0, time.FixedZone("UTC+7", 7*60*60))
	nextCheckAt := completedAt.Add(30 * time.Second)
	statusOK := http.StatusOK
	statusUnavailable := http.StatusServiceUnavailable
	settings := models.Target{ID: 1, URL: "https://example.com/health", Interval: 30 * time.Second}

	tests := []struct {
		name  string
		state models.TargetState
		want  string
	}{
		{
			name: "pending retains null fields",
			state: models.TargetState{
				Settings: settings, NextCheckAt: &nextCheckAt,
			},
			want: `{"id":1,"url":"https://example.com/health","interval_seconds":30,"status":"pending","last_checked_at":null,"status_code":null,"latency_ms":null,"error":null,"next_check_at":"2026-09-29T02:11:02Z"}`,
		},
		{
			name: "up converts timestamps and latency",
			state: models.TargetState{
				Settings: settings, NextCheckAt: &nextCheckAt,
				LastCheck: &models.CheckResult{
					Status: models.StatusUp, StatusCode: &statusOK,
					CompletedAt: completedAt, Latency: 143*time.Millisecond + 999*time.Microsecond,
				},
			},
			want: `{"id":1,"url":"https://example.com/health","interval_seconds":30,"status":"up","last_checked_at":"2026-09-29T02:10:32Z","status_code":200,"latency_ms":143,"error":null,"next_check_at":"2026-09-29T02:11:02Z"}`,
		},
		{
			name: "network error during next check",
			state: models.TargetState{
				Settings: settings,
				LastCheck: &models.CheckResult{
					Status: models.StatusDown, CompletedAt: completedAt,
					Latency: 3001 * time.Millisecond, Error: context.DeadlineExceeded,
				},
			},
			want: `{"id":1,"url":"https://example.com/health","interval_seconds":30,"status":"down","last_checked_at":"2026-09-29T02:10:32Z","status_code":null,"latency_ms":3001,"error":"context deadline exceeded","next_check_at":null}`,
		},
		{
			name: "http failure has a code and no error",
			state: models.TargetState{
				Settings: settings,
				LastCheck: &models.CheckResult{
					Status: models.StatusDown, StatusCode: &statusUnavailable,
					CompletedAt: completedAt, Latency: 82 * time.Millisecond,
				},
			},
			want: `{"id":1,"url":"https://example.com/health","interval_seconds":30,"status":"down","last_checked_at":"2026-09-29T02:10:32Z","status_code":503,"latency_ms":82,"error":null,"next_check_at":null}`,
		},
		{
			name: "zero latency is a value",
			state: models.TargetState{
				Settings: settings,
				LastCheck: &models.CheckResult{
					Status: models.StatusUp, StatusCode: &statusOK, CompletedAt: completedAt,
				},
			},
			want: `{"id":1,"url":"https://example.com/health","interval_seconds":30,"status":"up","last_checked_at":"2026-09-29T02:10:32Z","status_code":200,"latency_ms":0,"error":null,"next_check_at":null}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(newTargetResponse(tt.state))
			if err != nil {
				t.Fatal(err)
			}
			var got, want map[string]any
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tt.want), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("response = %s; want %s", data, tt.want)
			}
		})
	}
}

func TestTargetHandlersShareRepresentation(t *testing.T) {
	p := poller.New()
	mux := http.NewServeMux()
	mux.Handle("POST /targets", NewCreateTarget(p))
	mux.Handle("GET /targets/{id}", NewGetTarget(p))
	mux.Handle("GET /targets", NewGetTargets(p))

	request := httptest.NewRequest(http.MethodPost, "/targets", strings.NewReader(`{"url":"https://example.com/health","interval_seconds":30}`))
	request.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	mux.ServeHTTP(created, request)
	if created.Code != http.StatusCreated {
		t.Fatalf("POST status = %d; body = %s", created.Code, created.Body)
	}
	if location := created.Header().Get("Location"); location != "/targets/1" {
		t.Fatalf("Location = %q; want /targets/1", location)
	}
	if contentType := created.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q; want application/json", contentType)
	}

	var initial map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	if len(initial) != 9 || initial["status"] != "pending" || initial["next_check_at"] == nil {
		t.Fatalf("unexpected creation snapshot: %s", created.Body)
	}
	for _, key := range []string{"last_checked_at", "status_code", "latency_ms", "error"} {
		if value, exists := initial[key]; !exists || value != nil {
			t.Errorf("initial %s = %v, exists = %v; want null", key, value, exists)
		}
	}

	got := httptest.NewRecorder()
	mux.ServeHTTP(got, httptest.NewRequest(http.MethodGet, created.Header().Get("Location"), nil))
	if got.Code != http.StatusOK || got.Body.String() != created.Body.String() {
		t.Fatalf("GET status = %d; body = %s; want %s", got.Code, got.Body, created.Body)
	}

	if err := p.DeleteTarget(1); err != nil {
		t.Fatal(err)
	}
	listed := httptest.NewRecorder()
	mux.ServeHTTP(listed, httptest.NewRequest(http.MethodGet, "/targets", nil))
	if listed.Code != http.StatusOK || listed.Body.String() != `{"targets":[]}` {
		t.Fatalf("empty list status = %d; body = %s", listed.Code, listed.Body)
	}

	p.CreateTarget("https://example.com/second", time.Minute)
	p.CreateTarget("https://example.com/third", time.Minute)
	listed = httptest.NewRecorder()
	mux.ServeHTTP(listed, httptest.NewRequest(http.MethodGet, "/targets", nil))
	if listed.Code != http.StatusOK {
		t.Fatalf("list status = %d; body = %s", listed.Code, listed.Body)
	}
	var list struct {
		Targets []json.RawMessage `json:"targets"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Targets) != 2 {
		t.Fatalf("list length = %d; want 2", len(list.Targets))
	}
	for i, path := range []string{"/targets/2", "/targets/3"} {
		got = httptest.NewRecorder()
		mux.ServeHTTP(got, httptest.NewRequest(http.MethodGet, path, nil))
		if got.Code != http.StatusOK || got.Body.String() != string(list.Targets[i]) {
			t.Errorf("list item %d = %s; GET %s status = %d, body = %s", i, list.Targets[i], path, got.Code, got.Body)
		}
	}
}
