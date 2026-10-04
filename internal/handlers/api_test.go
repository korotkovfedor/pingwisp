package handlers_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/korotkovfedor/pingwisp/internal/checker"
	"github.com/korotkovfedor/pingwisp/internal/handlers"
	"github.com/korotkovfedor/pingwisp/internal/poller"
)

type apiResponse struct {
	status int
	header http.Header
	body   []byte
}

func newTestAPI(t *testing.T) (*poller.Poller, *http.Client) {
	t.Helper()
	p := poller.New(checker.NewHTTP(nil))
	// Leave the scheduler stopped so the initial pending state is deterministic.
	server := httptest.NewTestServer(t, handlers.NewRouter(p))
	return p, server.Client()
}

func requestAPI(t *testing.T, client *http.Client, method, path string, body io.Reader) apiResponse {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, "http://example.com"+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return apiResponse{status: resp.StatusCode, header: resp.Header, body: data}
}

func assertAPIError(t *testing.T, resp apiResponse, status int, code, field string) {
	t.Helper()
	if resp.status != status {
		t.Fatalf("status = %d; want %d; body = %s", resp.status, status, resp.body)
	}
	if got := resp.header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q; want application/json", got)
	}
	if location := resp.header.Get("Location"); location != "" {
		t.Errorf("error response contains Location: %s", location)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(resp.body, &envelope); err != nil {
		t.Fatalf("invalid JSON error: %s: %v", resp.body, err)
	}
	if len(envelope) != 1 || envelope["error"] == nil {
		t.Fatalf("unexpected error envelope: %s", resp.body)
	}
	var details map[string]string
	if err := json.Unmarshal(envelope["error"], &details); err != nil {
		t.Fatalf("invalid error details: %s: %v", resp.body, err)
	}
	if details["code"] != code || details["message"] == "" {
		t.Errorf("error = %s; want code %q and nonempty message", resp.body, code)
	}
	wantFields := 2
	if field != "" {
		wantFields++
		if details["field"] != field {
			t.Errorf("error field = %q; want %q", details["field"], field)
		}
	} else if _, ok := details["field"]; ok {
		t.Errorf("error field should be omitted: %s", resp.body)
	}
	if len(details) != wantFields {
		t.Errorf("unexpected error fields: %s", resp.body)
	}
}

func assertCreatedTarget(t *testing.T, resp apiResponse, interval int) {
	t.Helper()
	if resp.status != http.StatusCreated {
		t.Fatalf("POST status = %d; want 201; body = %s", resp.status, resp.body)
	}
	if resp.header.Get("Content-Type") != "application/json" || resp.header.Get("Location") != "/targets/1" {
		t.Errorf("unexpected creation headers: %v", resp.header)
	}
	var target struct {
		ID              uint64 `json:"id"`
		IntervalSeconds int    `json:"interval_seconds"`
		Status          string `json:"status"`
	}
	if err := json.Unmarshal(resp.body, &target); err != nil {
		t.Fatal(err)
	}
	if target.ID != 1 || target.IntervalSeconds != interval || target.Status != "pending" {
		t.Errorf("unexpected created target: %s", resp.body)
	}
}

func TestAPIIntervalBoundaries(t *testing.T) {
	for _, interval := range []int{1, 86400, 0, 86401} {
		t.Run(fmt.Sprint(interval), func(t *testing.T) {
			t.Parallel()
			p, client := newTestAPI(t)
			body := fmt.Sprintf(`{"url":"https://example.com/health","interval_seconds":%d}`, interval)
			resp := requestAPI(t, client, http.MethodPost, "/targets", strings.NewReader(body))
			if interval == 0 || interval == 86401 {
				assertAPIError(t, resp, http.StatusBadRequest, "invalid_request", "interval_seconds")
				if states := p.GetTargets(); len(states) != 0 {
					t.Errorf("invalid request created targets: %+v", states)
				}
				return
			}
			assertCreatedTarget(t, resp, interval)
			state, ok := p.GetTarget(1)
			if !ok || state.Settings.Interval != time.Duration(interval)*time.Second {
				t.Errorf("stored interval = %s, exists = %v; want %ds", state.Settings.Interval, ok, interval)
			}
		})
	}
}

func TestAPIRequestBodyLimit(t *testing.T) {
	const limit = 16 * 1024
	const prefix = `{"url":"https://example.com/`
	const suffix = `","interval_seconds":30}`
	makeBody := func(size int) string {
		return prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix
	}
	for _, tc := range []struct {
		name string
		body string
		ok   bool
	}{
		{"exact limit", makeBody(limit), true},
		{"one byte over", makeBody(limit + 1), false},
		{"exact limit including JSON tail", makeBody(100) + strings.Repeat(" ", limit-100), true},
		{"large tail after JSON", makeBody(100) + strings.Repeat(" ", 4*limit), false},
	} {
		for _, chunked := range []bool{false, true} {
			name := "content length"
			if chunked {
				name = "chunked"
			}
			t.Run(tc.name+"/"+name, func(t *testing.T) {
				t.Parallel()
				p, client := newTestAPI(t)
				var body io.Reader = strings.NewReader(tc.body)
				if chunked {
					// Hide the reader's length so net/http sends a chunked body.
					body = io.NopCloser(body)
				}
				resp := requestAPI(t, client, http.MethodPost, "/targets", body)
				if tc.ok {
					assertCreatedTarget(t, resp, 30)
					return
				}
				assertAPIError(t, resp, http.StatusRequestEntityTooLarge, "request_too_large", "")
				if states := p.GetTargets(); len(states) != 0 {
					t.Errorf("oversized request created targets: %+v", states)
				}
			})
		}
	}
}

func TestAPIInvalidJSONAndFieldTypes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		body  string
		code  string
		field string
	}{
		{"empty body", "", "invalid_json", ""},
		{"truncated JSON", `{"url":"https://example.com/health"`, "invalid_json", ""},
		{"invalid syntax", `{"url":}`, "invalid_json", ""},
		{"array instead of object", `[]`, "invalid_json", ""},
		{"numeric URL", `{"url":42,"interval_seconds":30}`, "invalid_request", "url"},
		{"object URL", `{"url":{},"interval_seconds":30}`, "invalid_request", "url"},
		{"string interval", `{"url":"https://example.com","interval_seconds":"30"}`, "invalid_request", "interval_seconds"},
		{"fractional interval", `{"url":"https://example.com","interval_seconds":1.5}`, "invalid_request", "interval_seconds"},
		{"boolean interval", `{"url":"https://example.com","interval_seconds":true}`, "invalid_request", "interval_seconds"},
		{"overflowing interval", `{"url":"https://example.com","interval_seconds":18446744073709551616}`, "invalid_request", "interval_seconds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, client := newTestAPI(t)
			resp := requestAPI(t, client, http.MethodPost, "/targets", strings.NewReader(tc.body))
			assertAPIError(t, resp, http.StatusBadRequest, tc.code, tc.field)
			if states := p.GetTargets(); len(states) != 0 {
				t.Errorf("invalid request created targets: %+v", states)
			}
		})
	}
}

func TestAPIInvalidTargetID(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		for _, id := range []string{"abc", "-1", "1.5", "18446744073709551616"} {
			t.Run(method+"/"+id, func(t *testing.T) {
				t.Parallel()
				p, client := newTestAPI(t)
				p.CreateTarget("https://example.com/health", time.Minute)
				resp := requestAPI(t, client, method, "/targets/"+id, nil)
				assertAPIError(t, resp, http.StatusBadRequest, "invalid_id", "id")
				if _, ok := p.GetTarget(1); !ok {
					t.Error("invalid ID request deleted an existing target")
				}
			})
		}
	}
}

func TestAPIDeleteTarget(t *testing.T) {
	p, client := newTestAPI(t)
	created := requestAPI(t, client, http.MethodPost, "/targets", strings.NewReader(`{"url":"https://example.com/health","interval_seconds":30}`))
	assertCreatedTarget(t, created, 30)
	path := created.header.Get("Location")
	deleted := requestAPI(t, client, http.MethodDelete, path, nil)
	if deleted.status != http.StatusNoContent || len(deleted.body) != 0 || deleted.header.Get("Content-Type") != "" {
		t.Fatalf("DELETE status = %d, headers = %v, body = %s; want empty 204", deleted.status, deleted.header, deleted.body)
	}
	if _, ok := p.GetTarget(1); ok {
		t.Error("deleted target remains in storage")
	}
	for _, method := range []string{http.MethodDelete, http.MethodGet} {
		resp := requestAPI(t, client, method, path, nil)
		assertAPIError(t, resp, http.StatusNotFound, "target_not_found", "")
	}
	listed := requestAPI(t, client, http.MethodGet, "/targets", nil)
	if listed.status != http.StatusOK || string(listed.body) != `{"targets":[]}` {
		t.Errorf("list after deletion: status = %d, body = %s", listed.status, listed.body)
	}
}

func TestAPIRouterErrors(t *testing.T) {
	for _, tc := range []struct {
		method string
		path   string
		status int
		code   string
		allow  string
	}{
		{http.MethodGet, "/", 404, "route_not_found", ""},
		{http.MethodPost, "/missing", 404, "route_not_found", ""},
		{http.MethodGet, "/targets/1/extra", 404, "route_not_found", ""},
		{http.MethodDelete, "/targets", 405, "method_not_allowed", "GET, HEAD, POST"},
		{http.MethodPut, "/targets", 405, "method_not_allowed", "GET, HEAD, POST"},
		{http.MethodOptions, "/targets", 405, "method_not_allowed", "GET, HEAD, POST"},
		{http.MethodPost, "/targets/1", 405, "method_not_allowed", "DELETE, GET, HEAD"},
		{http.MethodPut, "/targets/1", 405, "method_not_allowed", "DELETE, GET, HEAD"},
		{http.MethodOptions, "/targets/1", 405, "method_not_allowed", "DELETE, GET, HEAD"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			t.Parallel()
			_, client := newTestAPI(t)
			resp := requestAPI(t, client, tc.method, tc.path, nil)
			assertAPIError(t, resp, tc.status, tc.code, "")
			if allow := resp.header.Get("Allow"); allow != tc.allow {
				t.Errorf("Allow = %q; want %q", allow, tc.allow)
			}
		})
	}
}
