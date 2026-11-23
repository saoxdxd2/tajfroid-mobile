package lib

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type TursoClient struct {
	URL   string
	Token string
	HTTP  *http.Client
}

type tursoConfig struct {
	url       string
	tokenHash string
}

// One pooled client per configuration. A bare sync.Once would pin the first
// test server; keying by env values keeps local tests and token rotation safe.
var tursoClients sync.Map

func NewTurso() *TursoClient {
	url := os.Getenv("TURSO_DATABASE_URL")
	token := os.Getenv("TURSO_AUTH_TOKEN")
	// Convert libsql:// to https://
	if len(url) > 9 && url[:9] == "libsql://" {
		url = "https://" + url[9:]
	}
	cfg := tursoConfig{url: url, tokenHash: Sha256Hex(token)}
	if c, ok := tursoClients.Load(cfg); ok {
		return c.(*TursoClient)
	}

	// Keep-alives matter: the relay issues several Execute calls per request,
	// and a warm Vercel invocation can serve many requests. A fresh transport
	// per call forced a new TCP/TLS handshake every time and produced a large
	// p95 tail.
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DisableKeepAlives = false
	tr.ForceAttemptHTTP2 = true
	tr.MaxIdleConns = 128
	tr.MaxIdleConnsPerHost = 64
	tr.MaxConnsPerHost = 64
	tr.IdleConnTimeout = 90 * time.Second
	tr.TLSHandshakeTimeout = 5 * time.Second
	tr.ResponseHeaderTimeout = 10 * time.Second

	c := &TursoClient{
		URL:   url,
		Token: token,
		HTTP:  &http.Client{Timeout: 15 * time.Second, Transport: tr},
	}
	actual, _ := tursoClients.LoadOrStore(cfg, c)
	return actual.(*TursoClient)
}

type TursoArg struct {
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

type TursoStmt struct {
	SQL  string     `json:"sql"`
	Args []TursoArg `json:"args,omitempty"`
}

type TursoRequest struct {
	Type string    `json:"type"`
	Stmt TursoStmt `json:"stmt"`
}

type TursoPipelineBody struct {
	Requests []TursoRequest `json:"requests"`
}

type TursoColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type TursoResult struct {
	Columns          []TursoColumn   `json:"columns"`
	Rows             [][]interface{} `json:"rows"`
	AffectedRowCount int64           `json:"affected_row_count"`
}

type TursoResponse struct {
	Results []struct {
		Type     string `json:"type"`
		Response struct {
			Result TursoResult `json:"result"`
		} `json:"response"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"results"`
}

func (c *TursoClient) Execute(sql string, args ...TursoArg) (TursoResult, error) {
	return c.ExecuteContext(context.Background(), sql, args...)
}

func (c *TursoClient) ExecuteContext(ctx context.Context, sql string, args ...TursoArg) (TursoResult, error) {
	body := TursoPipelineBody{
		Requests: []TursoRequest{{Type: "execute", Stmt: TursoStmt{SQL: sql, Args: args}}},
	}
	tr, err := c.doPipeline(ctx, body, isReadOnlySQL(sql))
	if err != nil {
		return TursoResult{}, err
	}
	if len(tr.Results) == 0 {
		return TursoResult{}, errors.New("no results")
	}
	if tr.Results[0].Error != nil {
		return TursoResult{}, fmt.Errorf("sql error: %s", tr.Results[0].Error.Message)
	}
	res := tr.Results[0].Response.Result
	normalizeResult(&res)
	return res, nil
}

// ExecuteBatch sends several statements in one HTTPS round-trip. Results keep
// request order and every statement error is checked, not just the first.
func (c *TursoClient) ExecuteBatch(ctx context.Context, stmts ...TursoStmt) ([]TursoResult, error) {
	if len(stmts) == 0 {
		return nil, nil
	}
	reqs := make([]TursoRequest, 0, len(stmts))
	readOnly := true
	for _, stmt := range stmts {
		reqs = append(reqs, TursoRequest{Type: "execute", Stmt: stmt})
		if !isReadOnlySQL(stmt.SQL) {
			readOnly = false
		}
	}
	tr, err := c.doPipeline(ctx, TursoPipelineBody{Requests: reqs}, readOnly)
	if err != nil {
		return nil, err
	}
	if len(tr.Results) != len(stmts) {
		return nil, fmt.Errorf("pipeline returned %d results for %d statements", len(tr.Results), len(stmts))
	}
	out := make([]TursoResult, 0, len(stmts))
	for i := range tr.Results {
		if tr.Results[i].Error != nil {
			return nil, fmt.Errorf("sql error at %d: %s", i, tr.Results[i].Error.Message)
		}
		res := tr.Results[i].Response.Result
		normalizeResult(&res)
		out = append(out, res)
	}
	return out, nil
}

func (c *TursoClient) doPipeline(ctx context.Context, body TursoPipelineBody, readOnly bool) (TursoResponse, error) {
	b, _ := json.Marshal(body)
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/v2/pipeline", bytes.NewReader(b))
		if err != nil {
			return TursoResponse{}, err
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err
			// Never replay writes: a commit may have happened before the
			// connection failed. Reads are safe to retry immediately.
			if attempt == 0 && readOnly && isRetryable(err) {
				continue
			}
			return TursoResponse{}, err
		}
		raw, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return TursoResponse{}, readErr
		}
		var tr TursoResponse
		if err := json.Unmarshal(raw, &tr); err != nil {
			return TursoResponse{}, fmt.Errorf("parse: %w (%s)", err, string(raw))
		}
		return tr, nil
	}
	return TursoResponse{}, lastErr
}

func isReadOnlySQL(sql string) bool {
	s := strings.ToUpper(strings.TrimSpace(sql))
	return strings.HasPrefix(s, "SELECT") || strings.HasPrefix(s, "PRAGMA")
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe")
}

func normalizeResult(res *TursoResult) {
	for i, row := range res.Rows {
		for j, v := range row {
			row[j] = Unwrap(v)
		}
		res.Rows[i] = row
	}
}

func Text(v string) TursoArg { return TursoArg{Type: "text", Value: v} }
func Int(v int64) TursoArg   { return TursoArg{Type: "integer", Value: fmt.Sprintf("%d", v)} }
func Null() TursoArg         { return TursoArg{Type: "null"} }
