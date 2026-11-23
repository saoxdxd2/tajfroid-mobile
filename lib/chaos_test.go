package lib

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func chaosAssertPrintable(t *testing.T, got, input string) {
	t.Helper()
	if got == "" || len(got) > maxRequestIDLen {
		t.Fatalf("unsafe input %q produced id %q (len %d)", input, got, len(got))
	}
	for i := 0; i < len(got); i++ {
		if got[i] < 0x20 || got[i] > 0x7e {
			t.Fatalf("unsafe input %q produced non-printable id %q", input, got)
		}
	}
	if strings.ContainsAny(got, "\r\n") {
		t.Fatalf("unsafe input %q produced an injectable id %q", input, got)
	}
}

func TestChaosRequestIDIsAlwaysBoundedAndPrintable(t *testing.T) {
	hostile := []string{
		"safe\nX-Evil: 1",
		"a\r\nb",
		"a\x00b",
		"ünicode-id",
		"🙂emoji",
		strings.Repeat("x", 10*1024),
		"  ",
	}
	for _, in := range hostile {
		got := SafeInboundRequestID(in)
		chaosAssertPrintable(t, got, in)
		if trimmed := strings.TrimSpace(in); trimmed != "" && got == trimmed {
			t.Errorf("unsafe id %q was echoed unchanged", in)
		}
		line := string(EncodeRequestLog(RequestLog{
			Timestamp: "2026-11-24T00:00:00Z",
			RequestID: got,
			Route:     SafeRouteLabel(strings.Repeat("z", 10*1024)),
			Method:    SafeMethod("BREW\x00"),
			Status:    http.StatusBadRequest,
		}))
		if strings.Contains(line, "\n") || strings.Contains(line, "\x00") {
			t.Errorf("log line is not single-line/printable: %q", line)
		}
		var fields map[string]interface{}
		if err := json.Unmarshal([]byte(line), &fields); err != nil {
			t.Fatalf("bad log JSON for %q: %v", in, err)
		}
		if fields["requestId"] != got || fields["route"] != "unknown" || fields["method"] != "UNKNOWN" {
			t.Errorf("unexpected sanitised log fields: %s", line)
		}
	}

	valid := strings.Repeat("a", maxRequestIDLen)
	if got := SafeInboundRequestID(valid); got != valid {
		t.Errorf("128-byte printable id should be preserved, got %q", got)
	}
	if got := SafeMethod(strings.Repeat("M", 100)); got != "UNKNOWN" {
		t.Errorf("oversized method was not sanitised: %q", got)
	}
}

type chaosTursoMock struct {
	mu      sync.Mutex
	hashes  map[string]bool
	lookups int
	posts   int
	args    [][]TursoArg
}

func (m *chaosTursoMock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body TursoPipelineBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	results := make([]interface{}, 0, len(body.Requests))
	m.mu.Lock()
	m.posts++
	for _, req := range body.Requests {
		rows := [][]interface{}{}
		if strings.Contains(req.Stmt.SQL, "FROM agent_tokens WHERE token_hash = ?") {
			m.lookups++
			m.args = append(m.args, append([]TursoArg(nil), req.Stmt.Args...))
			if len(req.Stmt.Args) > 0 && m.hashes[ToString(req.Stmt.Args[0].Value)] {
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

func (m *chaosTursoMock) counts() (int, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lookups, m.posts
}

func (m *chaosTursoMock) argSnapshot() [][]TursoArg {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([][]TursoArg(nil), m.args...)
}

func TestChaosAuthBoundsTokensAndDoesOneLookup(t *testing.T) {
	const validToken = "chaos-raw-token-7c1f9a"
	mock := &chaosTursoMock{hashes: map[string]bool{Sha256Hex(validToken): true}}
	srv := httptest.NewServer(mock)
	defer srv.Close()
	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", "chaos-turso-secret")
	db := NewTurso()
	if tr, ok := db.HTTP.Transport.(*http.Transport); ok {
		defer tr.CloseIdleConnections()
	}

	cases := []struct {
		name       string
		header     string
		duplicate  string
		wantErr    error
		wantLookup int
	}{
		{name: "missing", wantErr: ErrMissingToken},
		{name: "malformed scheme", header: "Basic abc", wantErr: ErrMissingToken},
		{name: "scheme only", header: "Bearer ", wantErr: ErrMissingToken},
		{name: "oversized", header: "Bearer " + strings.Repeat("x", 10*1024), wantErr: ErrInvalidToken},
		{name: "control characters", header: "Bearer abc\x00def", wantErr: ErrInvalidToken},
		{name: "random", header: "Bearer random-token", wantErr: ErrInvalidToken, wantLookup: 1},
		{name: "valid", header: "Bearer " + validToken, wantLookup: 1},
		{name: "duplicate headers", header: "Bearer " + validToken, duplicate: "Bearer attacker-token", wantLookup: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beforeLookups, _ := mock.counts()
			req := httptest.NewRequest(http.MethodGet, "/api/invoices", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			if tc.duplicate != "" {
				req.Header.Add("Authorization", tc.duplicate)
			}
			id, err := Authenticate(req)
			gotLookups, _ := mock.counts()
			if err != tc.wantErr {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if got := gotLookups - beforeLookups; got != tc.wantLookup {
				t.Fatalf("token lookups = %d, want %d", got, tc.wantLookup)
			}
			if tc.wantErr == nil && (id.Kind != "device" || id.ID != "chaos-device") {
				t.Fatalf("identity = %#v", id)
			}
		})
	}

	for _, args := range mock.argSnapshot() {
		if len(args) < 2 {
			t.Fatalf("auth lookup has %d args, want 2", len(args))
		}
		for _, a := range args {
			v := ToString(a.Value)
			if strings.Contains(v, validToken) || strings.Contains(v, "attacker-token") {
				t.Fatal("raw bearer token was sent to the database")
			}
			if v != Sha256Hex(validToken) && v != Sha256Hex("random-token") {
				t.Errorf("unexpected auth argument %q", v)
			}
		}
	}
}

func TestChaosMetricsCardinalityUnderConcurrentWriters(t *testing.T) {
	m := NewLatencyMetrics()
	const routes = 600
	var writers sync.WaitGroup
	var snapshotter sync.WaitGroup

	stop := make(chan struct{})
	var snapshots int64
	snapshotter.Add(1)
	go func() {
		defer snapshotter.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = m.Snapshot()
				atomic.AddInt64(&snapshots, 1)
			}
		}
	}()

	for w := 0; w < 40; w++ {
		writers.Add(1)
		go func(worker int) {
			defer writers.Done()
			for i := worker; i < routes; i += 40 {
				m.Record("chaos-route-"+strconv.Itoa(i), http.StatusOK, time.Millisecond)
				m.Record("chaos-route-"+strconv.Itoa(i), http.StatusUnauthorized, 2*time.Millisecond)
			}
		}(w)
	}
	writers.Wait()
	close(stop)
	snapshotter.Wait()

	snap := m.Snapshot()
	if snap.Total != routes*2 {
		t.Fatalf("total samples = %d, want %d", snap.Total, routes*2)
	}
	if len(snap.Routes) > maxMetricKeys+1 {
		t.Fatalf("metric keys = %d, want <= %d", len(snap.Routes), maxMetricKeys+1)
	}
	if _, ok := snap.Routes[overflowMetricKey]; !ok {
		t.Error("overflow bucket missing after 600 unique routes")
	}
	for key := range snap.Routes {
		if key != overflowMetricKey && SafeRouteLabel(key) != key {
			t.Errorf("unsafe metric key %q", key)
		}
		if strings.ContainsAny(key, "\r\n\x00") || strings.Contains(key, "Bearer") {
			t.Errorf("hostile metric key %q", key)
		}
	}
	t.Logf("concurrent snapshots observed: %d", atomic.LoadInt64(&snapshots))
}

func TestChaosTursoMalformedResponseNeverLeaks(t *testing.T) {
	const secret = "chaos-turso-secret-in-body"

	cases := []struct {
		name    string
		payload []byte
	}{
		{
			name:    "invalid JSON with secret",
			payload: []byte(`{"results":[{"type":"execute",` + secret + `]}`),
		},
		{
			name:    "oversized invalid response",
			payload: []byte(`{"pad":"` + strings.Repeat("A", 17<<20) + `",` + secret + `}`),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(tc.payload)
			}))
			defer srv.Close()
			t.Setenv("TURSO_DATABASE_URL", srv.URL)
			t.Setenv("TURSO_AUTH_TOKEN", secret)
			db := NewTurso()
			if tr, ok := db.HTTP.Transport.(*http.Transport); ok {
				defer tr.CloseIdleConnections()
			}
			_, err := db.ExecuteContext(context.Background(), "SELECT 1")
			if err == nil {
				t.Fatal("malformed upstream response should fail")
			}
			if len(err.Error()) > 1024 {
				t.Fatalf("error is unbounded: %d bytes", len(err.Error()))
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("error leaked upstream body: %s", err)
			}
		})
	}
}
