package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"tajfroid-mobile/lib"
)

type chaosUpstream struct {
	mu         sync.Mutex
	hashes     map[string]bool
	lookups    int
	statements []string
	args       [][]lib.TursoArg
}

func (m *chaosUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body lib.TursoPipelineBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad pipeline", http.StatusBadRequest)
		return
	}
	results := make([]interface{}, 0, len(body.Requests))
	m.mu.Lock()
	for _, req := range body.Requests {
		m.statements = append(m.statements, req.Stmt.SQL)
		m.args = append(m.args, append([]lib.TursoArg(nil), req.Stmt.Args...))
		rows := [][]interface{}{}
		if strings.Contains(req.Stmt.SQL, "FROM agent_tokens WHERE token_hash = ?") {
			m.lookups++
			if len(req.Stmt.Args) > 0 && m.hashes[lib.ToString(req.Stmt.Args[0].Value)] {
				rows = [][]interface{}{{"device", "chaos-device"}}
			}
		}
		results = append(results, map[string]interface{}{
			"type": "execute",
			"response": map[string]interface{}{"result": map[string]interface{}{
				"columns": []interface{}{}, "rows": rows, "affected_row_count": 0,
			}},
		})
	}
	m.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"results": results})
}

func (m *chaosUpstream) lookupCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lookups
}

func (m *chaosUpstream) snapshot() (int, []string, [][]lib.TursoArg) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lookups, append([]string(nil), m.statements...), append([][]lib.TursoArg(nil), m.args...)
}

func chaosSetup(t *testing.T, tokens ...string) *chaosUpstream {
	t.Helper()
	m := &chaosUpstream{hashes: make(map[string]bool)}
	for _, token := range tokens {
		m.hashes[lib.Sha256Hex(token)] = true
	}
	srv := httptest.NewServer(m)
	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", "chaos-upstream-token")
	if tr, ok := lib.NewTurso().HTTP.Transport.(*http.Transport); ok {
		t.Cleanup(tr.CloseIdleConnections)
	}
	t.Cleanup(srv.Close)
	return m
}

func chaosDo(t *testing.T, method, target, token string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	Handler(rec, req)
	return rec
}

// chaosCaptureStdout reads the pipe concurrently so even a few hundred log
// lines cannot block the request path.
func chaosCaptureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan []byte, 1)
	go func() {
		out, _ := io.ReadAll(r)
		done <- out
	}()
	defer func() {
		os.Stdout = old
		_ = r.Close()
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return string(<-done)
}

func chaosAssertSafeRequestID(t *testing.T, got, input string) {
	t.Helper()
	if got == "" || len(got) > 128 {
		t.Fatalf("unsafe request id %q produced %q (len %d)", input, got, len(got))
	}
	for i := 0; i < len(got); i++ {
		if got[i] < 0x20 || got[i] > 0x7e {
			t.Fatalf("unsafe request id %q produced %q", input, got)
		}
	}
}

func TestChaosMalformedRoutesAndWeirdMethods(t *testing.T) {
	t.Setenv("TURSO_DATABASE_URL", "")
	t.Setenv("TURSO_AUTH_TOKEN", "")
	huge := strings.Repeat("a", 10*1024)
	cases := []struct {
		name, method, target string
		want                 int
	}{
		{"oversized __route", http.MethodGet, "/api/router?__route=" + url.QueryEscape(huge), http.StatusNotFound},
		{"NUL in __route", http.MethodGet, "/api/router?__route=" + url.QueryEscape("health\x00evil"), http.StatusNotFound},
		{"newline in __route", http.MethodGet, "/api/router?__route=" + url.QueryEscape("health\nevil"), http.StatusNotFound},
		{"traversal-looking __route", http.MethodGet, "/api/router?__route=..%2F..%2Fetc%2Fpasswd", http.StatusNotFound},
		{"repeated __route first public", http.MethodGet, "/api/router?__route=health&__route=invoices", http.StatusServiceUnavailable},
		{"repeated __route first protected", http.MethodGet, "/api/router?__route=invoices&__route=health", http.StatusNotFound},
		{"invalid percent escape", http.MethodGet, "/api/router?__route=%ZZ", http.StatusNotFound},
		{"unknown route", http.MethodGet, "/api/router?__route=does-not-exist", http.StatusNotFound},
		{"PROPFIND on health", "PROPFIND", "/api/router?__route=health", http.StatusServiceUnavailable},
		{"BREW on pair", "BREW", "/api/router?__route=pair", http.StatusMethodNotAllowed},
		{"PATCH on protected route", http.MethodPatch, "/api/router?__route=products/manage", http.StatusNotFound},
		{"empty __route", http.MethodGet, "/api/router?__route=&__route=health", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := chaosDo(t, tc.method, tc.target, "", nil)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), huge) {
				t.Error("raw oversized route was echoed")
			}
			chaosAssertSafeRequestID(t, rec.Header().Get(lib.RequestIDHeader), "")
		})
	}

	t.Run("hostile method is sanitised in logs", func(t *testing.T) {
		out := chaosCaptureStdout(t, func() {
			req := httptest.NewRequest(http.MethodGet, "/api/router?__route=health", nil)
			req.Method = "BAD\nMETHOD" + strings.Repeat("X", 200)
			rec := httptest.NewRecorder()
			Handler(rec, req)
		})
		if strings.Count(out, "\n") != 1 {
			t.Fatalf("method injected extra log lines: %q", out)
		}
		if !strings.Contains(out, `"method":"UNKNOWN"`) {
			t.Fatalf("hostile method was not sanitised: %q", out)
		}
	})

	// With a valid token the same hostile routes must fail closed with a
	// clean 404, and each request must do exactly one token lookup.
	const token = "chaos-route-token"
	mock := chaosSetup(t, token)
	before := mock.lookupCount()
	authed := []struct {
		method, target string
		want           int
	}{
		{http.MethodGet, "/api/router?__route=does-not-exist", http.StatusNotFound},
		{http.MethodGet, "/api/router?__route=" + url.QueryEscape(huge), http.StatusNotFound},
		{"PROPFIND", "/api/router?__route=other%00route", http.StatusNotFound},
	}
	for _, tc := range authed {
		rec := chaosDo(t, tc.method, tc.target, token, nil)
		if rec.Code != tc.want {
			t.Errorf("%s %s: status = %d, want %d", tc.method, tc.target, rec.Code, tc.want)
		}
	}
	if got := mock.lookupCount() - before; got != len(authed) {
		t.Fatalf("token lookups = %d, want %d", got, len(authed))
	}
}

func TestChaosRequestIDAbuseThroughRouter(t *testing.T) {
	t.Setenv("TURSO_DATABASE_URL", "")
	t.Setenv("TURSO_AUTH_TOKEN", "")
	cases := []struct {
		name     string
		values   []string
		wantEcho string
	}{
		{name: "valid", values: []string{"chaos-req-1"}, wantEcho: "chaos-req-1"},
		{name: "newline", values: []string{"safe\nX-Evil: 1"}},
		{name: "CRLF", values: []string{"a\r\nb"}},
		{name: "unicode", values: []string{"ünicode-🙂"}},
		{name: "oversized", values: []string{strings.Repeat("x", 10*1024)}},
		{name: "duplicate headers", values: []string{"first-safe", "second\r\nX-Evil: 1"}, wantEcho: "first-safe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := chaosCaptureStdout(t, func() {
				req := httptest.NewRequest(http.MethodGet, "/api/router?__route=health", nil)
				for i, v := range tc.values {
					if i == 0 {
						req.Header.Set(lib.RequestIDHeader, v)
					} else {
						req.Header.Add(lib.RequestIDHeader, v)
					}
				}
				rec := httptest.NewRecorder()
				Handler(rec, req)
				echo := rec.Header().Get(lib.RequestIDHeader)
				chaosAssertSafeRequestID(t, echo, tc.values[0])
				if tc.wantEcho != "" && echo != tc.wantEcho {
					t.Errorf("echo = %q, want %q", echo, tc.wantEcho)
				}
				if tc.wantEcho == "" && echo == tc.values[0] {
					t.Errorf("hostile id %q was echoed unchanged", tc.values[0])
				}
			})
			if strings.Count(out, "\n") != 1 {
				t.Fatalf("expected exactly one log line, got %q", out)
			}
			var entry lib.RequestLog
			if err := json.Unmarshal([]byte(strings.TrimSuffix(out, "\n")), &entry); err != nil {
				t.Fatalf("log is not a single JSON object: %q (%v)", out, err)
			}
			if tc.wantEcho == "" && (entry.RequestID == tc.values[0] || entry.RequestID == strings.TrimSpace(tc.values[0])) {
				t.Errorf("log echoed hostile id: %q", out)
			}
			if strings.Contains(out, "X-Evil") || strings.Contains(out, strings.Repeat("x", 1000)) {
				t.Errorf("hostile request id leaked into log: %q", out)
			}
		})
	}
}

func TestChaosAuthAbuseDoesOneLookupAndNeverLeaks(t *testing.T) {
	const (
		tokenA = "chaos-auth-token-a"
		tokenB = "chaos-auth-token-b"
	)
	mock := chaosSetup(t, tokenA, tokenB)
	target := "/api/router?__route=agent/scans"

	sequential := []struct {
		name, header, token string
		want                int
		lookups             int
	}{
		{name: "missing", want: http.StatusNotFound},
		{name: "malformed", header: "Basic abc", want: http.StatusNotFound},
		{name: "oversized", header: "Bearer " + strings.Repeat("z", 10*1024), want: http.StatusNotFound},
		{name: "valid replay 1", token: tokenA, want: http.StatusOK, lookups: 1},
		{name: "valid replay 2", token: tokenA, want: http.StatusOK, lookups: 1},
		{name: "other valid token", token: tokenB, want: http.StatusOK, lookups: 1},
		{name: "random token", token: "chaos-random-token", want: http.StatusNotFound, lookups: 1},
	}
	for _, tc := range sequential {
		before := mock.lookupCount()
		var rec *httptest.ResponseRecorder
		if tc.header != "" {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			req.Header.Set("Authorization", tc.header)
			rec = httptest.NewRecorder()
			Handler(rec, req)
		} else {
			rec = chaosDo(t, http.MethodGet, target, tc.token, nil)
		}
		if rec.Code != tc.want {
			t.Errorf("%s: status = %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body.String())
		}
		if got := mock.lookupCount() - before; got != tc.lookups {
			t.Errorf("%s: token lookups = %d, want %d", tc.name, got, tc.lookups)
		}
		if strings.Contains(rec.Body.String(), tokenA) || strings.Contains(rec.Body.String(), tokenB) {
			t.Errorf("%s: response leaked a bearer token: %s", tc.name, rec.Body.String())
		}
	}

	// Concurrent callers with different tokens: one lookup per request, none
	// of the raw tokens in logs, SQL text or arguments.
	const concurrent = 60
	var wg sync.WaitGroup
	out := chaosCaptureStdout(t, func() {
		before := mock.lookupCount()
		for i := 0; i < concurrent; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				token := tokenA
				want := http.StatusOK
				if i%2 == 1 {
					token = "chaos-random-" + string(rune('a'+i%26))
					want = http.StatusNotFound
				}
				rec := chaosDo(t, http.MethodGet, target, token, nil)
				if rec.Code != want {
					t.Errorf("concurrent request %d: status = %d, want %d", i, rec.Code, want)
				}
			}(i)
		}
		wg.Wait()
		if got := mock.lookupCount() - before; got != concurrent {
			t.Errorf("concurrent token lookups = %d, want %d", got, concurrent)
		}
	})
	if strings.Contains(out, tokenA) || strings.Contains(out, tokenB) || strings.Contains(out, "Bearer ") {
		t.Errorf("stdout request log leaked a token: %s", out)
	}

	_, statements, args := mock.snapshot()
	for _, sql := range statements {
		if strings.Contains(sql, tokenA) || strings.Contains(sql, tokenB) {
			t.Errorf("token appeared in SQL text: %s", sql)
		}
	}
	for _, group := range args {
		for _, a := range group {
			v := lib.ToString(a.Value)
			if strings.Contains(v, tokenA) || strings.Contains(v, tokenB) {
				t.Error("raw bearer token appeared in a SQL argument")
			}
		}
	}
}

func TestChaosAuthDatabaseErrorIsNotEchoed(t *testing.T) {
	const secret = "chaos-upstream-secret-detail"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"error":{"message":"` + secret + `"}}]}`))
	}))
	defer srv.Close()
	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", secret)
	if tr, ok := lib.NewTurso().HTTP.Transport.(*http.Transport); ok {
		defer tr.CloseIdleConnections()
	}

	req := httptest.NewRequest(http.MethodGet, "/api/router?__route=invoices", nil)
	req.Header.Set("Authorization", "Bearer chaos-valid-looking-token")
	rec := httptest.NewRecorder()
	Handler(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), secret) {
		t.Fatalf("database error leaked to client: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "parse database response") {
		t.Fatalf("internal parser detail leaked: %s", rec.Body.String())
	}
}

func TestChaosJSONBodiesStayClean(t *testing.T) {
	const (
		validToken = "chaos-body-token"
		sqlPayload = "' OR 1=1; DROP TABLE devices; --"
	)
	mock := chaosSetup(t, validToken)
	huge := []byte(`{"pairingCode":"` + strings.Repeat("A", (20<<20)+1024) + `"}`)

	bodyCases := []struct {
		name, target, token, method string
		body                        []byte
		want                        int
	}{
		{"pair oversized", "/api/router?__route=pair", "", http.MethodPost, huge, http.StatusRequestEntityTooLarge},
		{"pair invalid JSON", "/api/router?__route=pair", "", http.MethodPost, []byte("{"), http.StatusBadRequest},
		{"pair wrong types", "/api/router?__route=pair", "", http.MethodPost, []byte(`{"pairingCode":123,"deviceName":[]}`), http.StatusBadRequest},
		{"pair null", "/api/router?__route=pair", "", http.MethodPost, []byte("null"), http.StatusBadRequest},
		{"pair SQL-looking", "/api/router?__route=pair", "", http.MethodPost, []byte(`{"pairingCode":"` + sqlPayload + `"}`), http.StatusUnauthorized},
		{"scans oversized", "/api/router?__route=scans", validToken, http.MethodPost, huge, http.StatusRequestEntityTooLarge},
		{"scans invalid JSON", "/api/router?__route=scans", validToken, http.MethodPost, []byte("{"), http.StatusBadRequest},
		{"scans wrong types", "/api/router?__route=scans", validToken, http.MethodPost, []byte(`{"clientId":{"nested":true}}`), http.StatusBadRequest},
		{"scans null", "/api/router?__route=scans", validToken, http.MethodPost, []byte("null"), http.StatusBadRequest},
		{"scans SQL-looking", "/api/router?__route=scans", validToken, http.MethodPost, []byte(`{"clientId":"` + sqlPayload + `"}`), http.StatusCreated},
	}
	for _, tc := range bodyCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := chaosDo(t, tc.method, tc.target, tc.token, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.want, rec.Body.String())
			}
			if rec.Code >= 500 {
				t.Fatalf("client body caused a 5xx: %s", rec.Body.String())
			}
		})
	}

	_, statements, args := mock.snapshot()
	for _, sql := range statements {
		if strings.Contains(sql, sqlPayload) {
			t.Errorf("SQL-looking user input was interpolated into SQL: %s", sql)
		}
	}
	parameterised := false
	for _, group := range args {
		for _, a := range group {
			if lib.ToString(a.Value) == sqlPayload {
				parameterised = true
			}
		}
	}
	if !parameterised {
		t.Error("SQL-looking body value was never sent as a bound parameter")
	}
}

func TestChaosMetricsCardinalityThroughRouter(t *testing.T) {
	t.Setenv("OBSERVABILITY_METRICS", "1")
	t.Setenv("TURSO_DATABASE_URL", "")
	t.Setenv("TURSO_AUTH_TOKEN", "")
	// 520 unique routes would evict other tests' keys from the shared window.
	// Isolate the global metrics object and restore it afterwards.
	previous := lib.DefaultMetrics
	lib.DefaultMetrics = lib.NewLatencyMetrics()
	defer func() { lib.DefaultMetrics = previous }()

	const routes = 520
	out := chaosCaptureStdout(t, func() {
		for i := 0; i < routes; i++ {
			target := "/api/router?__route=chaos-unique-" + strconv.Itoa(i)
			rec := chaosDo(t, http.MethodGet, target, "", nil)
			if rec.Code != http.StatusNotFound {
				t.Errorf("request %d: status = %d, want 404", i, rec.Code)
			}
		}
	})
	if lines := strings.Count(out, "\n"); lines != routes {
		t.Errorf("log lines = %d, want %d", lines, routes)
	}
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if len(line) > 512 {
			t.Errorf("oversized log line: %d bytes", len(line))
		}
	}
	if !strings.Contains(out, "chaos-unique-519") {
		t.Error("safe route label missing from logs")
	}

	rec := chaosDo(t, http.MethodGet, "/api/router?__route=metrics", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var snap lib.MetricsSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.Routes) > 257 {
		t.Fatalf("metric keys = %d, want <= 257", len(snap.Routes))
	}
	if snap.Total < routes {
		t.Errorf("total samples = %d, want >= %d", snap.Total, routes)
	}
	for key := range snap.Routes {
		if key != "overflow" && lib.SafeRouteLabel(key) != key {
			t.Errorf("unsafe metric key %q", key)
		}
		if len(key) > 128 || strings.ContainsAny(key, "\r\n\x00") || strings.Contains(key, "Bearer") {
			t.Errorf("hostile metric key %q", key)
		}
	}
}
