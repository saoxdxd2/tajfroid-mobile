package lib

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync/atomic"
	"testing"
	"time"
)

func tursoOKPayload(results int) []byte {
	items := make([]interface{}, 0, results)
	for i := 0; i < results; i++ {
		items = append(items, map[string]interface{}{
			"type": "execute",
			"response": map[string]interface{}{
				"result": map[string]interface{}{
					"columns": []interface{}{}, "rows": []interface{}{},
					"affected_row_count": 0,
				},
			},
		})
	}
	b, _ := json.Marshal(map[string]interface{}{"results": items})
	return b
}

// TestWarmExecuteLatencyAndConnectionReuse is both a regression guard and a
// measurable local proxy for relay consistency. Before pooling, connection
// count equalled query count and a 50ms sleep sat on the retry path.
func TestWarmExecuteLatencyAndConnectionReuse(t *testing.T) {
	var posts, newConns int64
	payload := tursoOKPayload(1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&posts, 1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(payload)
	}))
	defer srv.Close()
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			atomic.AddInt64(&newConns, 1)
		}
	}

	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", "latency-test")
	db := NewTurso()
	transport, ok := db.HTTP.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport: %T", db.HTTP.Transport)
	}
	defer transport.CloseIdleConnections()

	ctx := context.Background()
	for i := 0; i < 10; i++ { // warm the pool before measuring
		if _, err := db.ExecuteContext(ctx, "SELECT 1"); err != nil {
			t.Fatal(err)
		}
	}

	const n = 500
	durations := make([]time.Duration, 0, n)
	for i := 0; i < n; i++ {
		start := time.Now()
		if _, err := db.ExecuteContext(ctx, "SELECT 1"); err != nil {
			t.Fatal(err)
		}
		durations = append(durations, time.Since(start))
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p50 := durations[len(durations)/2]
	p95 := durations[(len(durations)*95)/100]
	max := durations[len(durations)-1]

	gotPosts := atomic.LoadInt64(&posts)
	gotConns := atomic.LoadInt64(&newConns)
	t.Logf("warm Execute n=%d posts=%d newConns=%d p50=%s p95=%s max=%s",
		n, gotPosts, gotConns, p50, p95, max)
	if gotPosts != n+10 {
		t.Fatalf("posts=%d, want %d", gotPosts, n+10)
	}
	if gotConns > 2 {
		t.Fatalf("newConns=%d, want <=2 (keep-alive pool not reused)", gotConns)
	}
}

func TestExecuteBatchUsesOneRequest(t *testing.T) {
	var posts, requests int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&posts, 1)
		var body TursoPipelineBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		atomic.StoreInt64(&requests, int64(len(body.Requests)))
		w.Header().Set("Content-Type", "application/json")
		w.Write(tursoOKPayload(len(body.Requests)))
	}))
	defer srv.Close()

	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", "batch-test")
	db := NewTurso()
	if tr, ok := db.HTTP.Transport.(*http.Transport); ok {
		defer tr.CloseIdleConnections()
	}
	stmts := make([]TursoStmt, 200)
	for i := range stmts {
		stmts[i] = TursoStmt{SQL: "SELECT 1"}
	}
	results, err := db.ExecuteBatch(context.Background(), stmts...)
	if err != nil {
		t.Fatal(err)
	}
	gotPosts := atomic.LoadInt64(&posts)
	gotRequests := atomic.LoadInt64(&requests)
	t.Logf("batch: posts=%d statements=%d results=%d", gotPosts, gotRequests, len(results))
	if gotPosts != 1 {
		t.Fatalf("posts=%d, want 1", gotPosts)
	}
	if gotRequests != int64(len(stmts)) || len(results) != len(stmts) {
		t.Fatalf("requests=%d results=%d want %d", gotRequests, len(results), len(stmts))
	}
}

func TestWriteIsNotRetriedAfterTransportFailure(t *testing.T) {
	var posts int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&posts, 1)
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("hijacker unavailable")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		conn.Close()
	}))
	defer srv.Close()

	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", "retry-test")
	db := NewTurso()
	if tr, ok := db.HTTP.Transport.(*http.Transport); ok {
		defer tr.CloseIdleConnections()
	}
	if _, err := db.Execute("INSERT INTO t (id) VALUES (1)"); err == nil {
		t.Fatal("write should fail on transport error")
	}
	gotPosts := atomic.LoadInt64(&posts)
	t.Logf("write transport failure: posts=%d", gotPosts)
	if gotPosts != 1 {
		t.Fatalf("write posts=%d, want 1 (write was replayed)", gotPosts)
	}
}
