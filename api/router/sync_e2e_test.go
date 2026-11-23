package handler_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	handler "tajfroid-mobile/api/router"
	"tajfroid-mobile/lib"
)

// syncE2ETursoMock is a stateful Turso /v2/pipeline stub. It implements only
// the statements the real router + customers handlers issue, so the test
// exercises the full HTTP path (router auth -> handler -> lib.TursoClient)
// without depending on a live database.
type syncE2ETursoMock struct {
	mu sync.Mutex

	tokenHash string // SHA-256 of the raw bearer token used by the test
	deviceID  string
	customers map[string]*syncE2ECustomerRow

	statements   []syncE2EStatement
	authHeaders  []string
	tokenLookups int
	inserts      int
}

type syncE2EStatement struct {
	SQL  string
	Args []lib.TursoArg
}

type syncE2ECustomerRow struct {
	id, partyType, companyName                                string
	firstName, lastName, contactName, email, phone, iceNumber string
	isArchived                                                int64
	createdAt, updatedAt                                      int64
}

func (s *syncE2ETursoMock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v2/pipeline" {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.authHeaders = append(s.authHeaders, r.Header.Get("Authorization"))
	var body lib.TursoPipelineBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	results := make([]interface{}, 0, len(body.Requests))
	for _, req := range body.Requests {
		s.statements = append(s.statements, syncE2EStatement{SQL: req.Stmt.SQL, Args: req.Stmt.Args})
		rows := s.execute(req.Stmt)
		results = append(results, map[string]interface{}{
			"type": "execute",
			"response": map[string]interface{}{
				"result": map[string]interface{}{
					"columns":            []interface{}{},
					"rows":               rows,
					"affected_row_count": 1,
				},
			},
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"results": results})
}

// execute is called with s.mu held. Unknown statements answer with an empty
// result set, which keeps the mock honest: anything the handlers actually need
// must be implemented explicitly here.
func (s *syncE2ETursoMock) execute(stmt lib.TursoStmt) [][]interface{} {
	sql := stmt.SQL
	arg := func(i int) string {
		if i >= len(stmt.Args) {
			return ""
		}
		return lib.ToString(stmt.Args[i].Value)
	}

	switch {
	case strings.Contains(sql, "FROM agent_tokens WHERE token_hash = ?"):
		// lib.Authenticate hashes the bearer token before it ever reaches
		// Turso; only the hash should be visible on the wire.
		s.tokenLookups++
		if arg(0) == s.tokenHash {
			return [][]interface{}{{"device", s.deviceID}}
		}
		return [][]interface{}{}

	case strings.Contains(sql, "SELECT id, company_name FROM customers WHERE id=?"):
		// Idempotency pre-check in POST /api/customers.
		if c, ok := s.customers[arg(0)]; ok {
			return [][]interface{}{{c.id, c.companyName}}
		}
		return [][]interface{}{}

	case strings.Contains(sql, "SELECT updated_at FROM customers WHERE id=?"):
		// Optimistic-concurrency token read in PUT /api/customers/manage.
		if c, ok := s.customers[arg(0)]; ok {
			return [][]interface{}{{c.updatedAt}}
		}
		return [][]interface{}{}

	case strings.Contains(sql, "FROM customers WHERE id = ?"):
		// GET /api/customers/manage: 11 columns in handler order.
		c, ok := s.customers[arg(0)]
		if !ok {
			return [][]interface{}{}
		}
		return [][]interface{}{{
			c.id, c.partyType, c.companyName,
			syncE2ENullable(c.firstName), syncE2ENullable(c.lastName),
			syncE2ENullable(c.contactName), syncE2ENullable(c.email),
			syncE2ENullable(c.phone), syncE2ENullable(c.iceNumber),
			c.isArchived, c.updatedAt,
		}}

	case strings.HasPrefix(sql, "INSERT INTO customers"):
		s.inserts++
		c := &syncE2ECustomerRow{
			id:          arg(0),
			partyType:   arg(1),
			companyName: arg(2),
			firstName:   arg(3),
			lastName:    arg(4),
			contactName: arg(5),
			email:       arg(6),
			phone:       arg(7),
			iceNumber:   arg(8),
		}
		c.createdAt, _ = strconv.ParseInt(arg(10), 10, 64)
		c.updatedAt, _ = strconv.ParseInt(arg(11), 10, 64)
		s.customers[c.id] = c
		return [][]interface{}{}

	case strings.HasPrefix(sql, "UPDATE customers SET updated_at=?"):
		// A real Turso stores time.Now().Unix() verbatim. The whole test can
		// run inside one second, which would leave the version unchanged and
		// make a stale write look current. Synthesize a strictly increasing
		// version so the concurrency invariant is actually exercised.
		if c, ok := s.customers[arg(1)]; ok {
			v, err := strconv.ParseInt(arg(0), 10, 64)
			if err != nil || v <= c.updatedAt {
				v = c.updatedAt + 1
			}
			c.updatedAt = v
		}
		return [][]interface{}{}

	case strings.HasPrefix(sql, "UPDATE customers SET "):
		col := strings.TrimSuffix(strings.TrimPrefix(sql, "UPDATE customers SET "), "=? WHERE id=?")
		if c, ok := s.customers[arg(1)]; ok {
			syncE2ESetField(c, col, arg(0))
		}
		return [][]interface{}{}
	}
	return [][]interface{}{}
}

func syncE2ENullable(v string) interface{} {
	if v == "" {
		return nil
	}
	return v
}

func syncE2ESetField(c *syncE2ECustomerRow, col, value string) {
	switch col {
	case "party_type":
		c.partyType = value
	case "company_name":
		c.companyName = value
	case "first_name":
		c.firstName = value
	case "last_name":
		c.lastName = value
	case "contact_name":
		c.contactName = value
	case "email":
		c.email = value
	case "phone":
		c.phone = value
	case "ice_number":
		c.iceNumber = value
	case "is_archived":
		c.isArchived, _ = strconv.ParseInt(value, 10, 64)
	}
}

func (s *syncE2ETursoMock) tokenLookupCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokenLookups
}

func (s *syncE2ETursoMock) insertCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inserts
}

func (s *syncE2ETursoMock) allStatements() []syncE2EStatement {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]syncE2EStatement(nil), s.statements...)
}

func (s *syncE2ETursoMock) authHeaderSnapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.authHeaders...)
}

func (s *syncE2ETursoMock) customer(id string) syncE2ECustomerRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.customers[id]; ok {
		return *c
	}
	return syncE2ECustomerRow{}
}

// syncE2EBuffer captures the standard logger so a future log.Printf of an
// Authorization header would fail the no-leak assertion below.
type syncE2EBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncE2EBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *syncE2EBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

type syncE2EResult struct {
	status int
	body   string
	json   map[string]interface{}
}

// syncE2ECall issues one request through the real router and asserts the
// per-request auth invariant: exactly wantLookups token-lookup SQL statements.
func syncE2ECall(t *testing.T, mock *syncE2ETursoMock, method, target string, payload interface{}, token string, wantLookups int) syncE2EResult {
	t.Helper()
	var reader io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, target, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	before := mock.tokenLookupCount()
	rec := httptest.NewRecorder()
	handler.Handler(rec, req)
	if got := mock.tokenLookupCount() - before; got != wantLookups {
		t.Fatalf("%s %s: token lookup SQL = %d, want %d", method, target, got, wantLookups)
	}

	var obj map[string]interface{}
	if len(rec.Body.Bytes()) > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &obj) // plain-text http.Error bodies stay nil
	}
	return syncE2EResult{status: rec.Code, body: rec.Body.String(), json: obj}
}

func syncE2EInt64(t *testing.T, obj map[string]interface{}, key string) int64 {
	t.Helper()
	v, ok := obj[key]
	if !ok {
		t.Fatalf("response missing %q: %#v", key, obj)
	}
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case json.Number:
		n, err := x.Int64()
		if err != nil {
			t.Fatalf("bad %q: %v", key, err)
		}
		return n
	default:
		t.Fatalf("%q = %#v, want number", key, v)
		return 0
	}
}

func TestSyncE2ERelayOfflineConflictInvariants(t *testing.T) {
	const (
		rawToken   = "sync-e2e-raw-bearer-9f3a7c2e4b"
		tursoToken = "sync-e2e-turso-secret-not-the-bearer"
		customerID = "sync-e2e-customer-1"
	)

	mock := &syncE2ETursoMock{
		tokenHash: lib.Sha256Hex(rawToken),
		deviceID:  "sync-e2e-device-1",
		customers: make(map[string]*syncE2ECustomerRow),
	}
	srv := httptest.NewServer(mock)
	defer srv.Close()
	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", tursoToken)
	if tr, ok := lib.NewTurso().HTTP.Transport.(*http.Transport); ok {
		defer tr.CloseIdleConnections()
	}

	logs := &syncE2EBuffer{}
	prevLog := log.Writer()
	log.SetOutput(logs)
	defer log.SetOutput(prevLog)

	var responses []syncE2EResult

	// 1. Protected routes reject a missing token before touching the database.
	r := syncE2ECall(t, mock, http.MethodGet, "/api/customers", nil, "", 0)
	responses = append(responses, r)
	if r.status != http.StatusUnauthorized {
		t.Fatalf("missing token: got %d, want 401 (%s)", r.status, r.body)
	}

	// 2. Client-supplied id create; an identical replay must be idempotent.
	create := map[string]interface{}{
		"id": customerID, "partyType": "enterprise", "companyName": "Sync E2E Customer",
	}
	r = syncE2ECall(t, mock, http.MethodPost, "/api/customers", create, rawToken, 1)
	responses = append(responses, r)
	if r.status != http.StatusCreated || r.json["id"] != customerID {
		t.Fatalf("create: got %d %s", r.status, r.body)
	}
	if got := mock.insertCount(); got != 1 {
		t.Fatalf("inserts after create = %d, want 1", got)
	}

	r = syncE2ECall(t, mock, http.MethodPost, "/api/customers", create, rawToken, 1)
	responses = append(responses, r)
	if r.status != http.StatusOK || r.json["idempotent"] != true || r.json["id"] != customerID {
		t.Fatalf("idempotent replay: got %d %s", r.status, r.body)
	}
	if got := mock.insertCount(); got != 1 {
		t.Fatalf("inserts after replay = %d, want 1 (duplicate insert)", got)
	}

	manageTarget := "/api/customers/manage?id=" + url.QueryEscape(customerID)
	r = syncE2ECall(t, mock, http.MethodGet, manageTarget, nil, rawToken, 1)
	responses = append(responses, r)
	base := syncE2EInt64(t, r.json, "updatedAt")

	// 3. Correct base update succeeds; replaying that stale base must conflict.
	update := func(base int64, company string) map[string]interface{} {
		return map[string]interface{}{"companyName": company, "baseUpdatedAt": base}
	}
	r = syncE2ECall(t, mock, http.MethodPut, manageTarget, update(base, "Sync E2E Customer Updated"), rawToken, 1)
	responses = append(responses, r)
	if r.status != http.StatusOK || r.json["ok"] != true {
		t.Fatalf("update with current base: got %d %s", r.status, r.body)
	}

	r = syncE2ECall(t, mock, http.MethodPut, manageTarget, update(base, "Sync E2E Customer Stale"), rawToken, 1)
	responses = append(responses, r)
	if r.status != http.StatusConflict {
		t.Fatalf("stale base replay: got %d, want 409 (%s)", r.status, r.body)
	}
	conflictToken := syncE2EInt64(t, r.json, "currentUpdatedAt")
	if conflictToken <= base {
		t.Fatalf("conflict currentUpdatedAt = %d, want > base %d", conflictToken, base)
	}
	if errText, _ := r.json["error"].(string); !strings.Contains(errText, "conflict") {
		t.Fatalf("conflict body has no error marker: %s", r.body)
	}
	// Handler shape: lib.WriteConflict returns only {error, currentUpdatedAt};
	// it does not embed a full `record`. The closest reliable mergeable
	// invariant is therefore the current-version token above plus the server
	// record fetched in step 4, which is what a client would merge against.

	// 4. Fetch the current record and retry with its version as the new base.
	r = syncE2ECall(t, mock, http.MethodGet, manageTarget, nil, rawToken, 1)
	responses = append(responses, r)
	if got := syncE2EInt64(t, r.json, "updatedAt"); got != conflictToken {
		t.Fatalf("GET updatedAt = %d, want conflict token %d", got, conflictToken)
	}
	if r.json["companyName"] != "Sync E2E Customer Updated" {
		t.Fatalf("merge source record = %#v", r.json)
	}
	current := syncE2EInt64(t, r.json, "updatedAt")
	r = syncE2ECall(t, mock, http.MethodPut, manageTarget, update(current, "Sync E2E Customer Merged"), rawToken, 1)
	responses = append(responses, r)
	if r.status != http.StatusOK || r.json["ok"] != true {
		t.Fatalf("retry with refreshed base: got %d %s", r.status, r.body)
	}
	if got := mock.customer(customerID).companyName; got != "Sync E2E Customer Merged" {
		t.Fatalf("stored companyName = %q, want merged value", got)
	}
	if got := mock.insertCount(); got != 1 {
		t.Fatalf("inserts after full sequence = %d, want 1", got)
	}

	// 5. The raw bearer token must never be logged or returned, and must never
	// reach Turso (only its SHA-256 hash is used in the auth lookup).
	for i, res := range responses {
		if strings.Contains(res.body, rawToken) {
			t.Errorf("response %d returned the raw bearer token: %s", i, res.body)
		}
	}
	for i, stmt := range mock.allStatements() {
		if strings.Contains(stmt.SQL, rawToken) {
			t.Errorf("statement %d contains the raw bearer token: %s", i, stmt.SQL)
		}
		for _, a := range stmt.Args {
			if strings.Contains(lib.ToString(a.Value), rawToken) {
				t.Errorf("statement %d argument contains the raw bearer token", i)
			}
		}
	}
	for _, h := range mock.authHeaderSnapshot() {
		if strings.Contains(h, rawToken) {
			t.Error("raw bearer token was sent to Turso in an Authorization header")
		}
	}
	if strings.Contains(logs.String(), rawToken) {
		t.Error("raw bearer token appeared in the standard logger output")
	}
}

func TestSyncE2EStdoutRequestLogHasNoSecrets(t *testing.T) {
	const (
		rawToken   = "sync-e2e-stdout-bearer-2b8d4f6a"
		tursoToken = "sync-e2e-stdout-turso-secret"
	)
	mock := &syncE2ETursoMock{
		tokenHash: lib.Sha256Hex(rawToken),
		deviceID:  "sync-e2e-stdout-device",
		customers: make(map[string]*syncE2ECustomerRow),
	}
	srv := httptest.NewServer(mock)
	defer srv.Close()
	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", tursoToken)
	if tr, ok := lib.NewTurso().HTTP.Transport.(*http.Transport); ok {
		defer tr.CloseIdleConnections()
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prevStdout := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = prevStdout
		reader.Close()
	}()

	res := syncE2ECall(t, mock, http.MethodGet, "/api/customers", nil, rawToken, 1)
	writer.Close()
	os.Stdout = prevStdout
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	if res.status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", res.status, res.body)
	}
	line := string(out)
	if strings.Contains(line, rawToken) || strings.Contains(line, tursoToken) {
		t.Errorf("stdout request log leaked a secret: %s", line)
	}
	if !strings.Contains(line, `"auth_kind":"device"`) {
		t.Errorf("stdout request log missing auth kind: %s", line)
	}
	if !strings.Contains(line, `"error_category"`) && !strings.Contains(line, `"status":200`) {
		t.Errorf("unexpected stdout request log: %s", line)
	}
}
