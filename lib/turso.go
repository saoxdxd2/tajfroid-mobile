package lib

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type TursoClient struct {
	URL   string
	Token string
	HTTP  *http.Client
}

func NewTurso() *TursoClient {
	url := os.Getenv("TURSO_DATABASE_URL")
	token := os.Getenv("TURSO_AUTH_TOKEN")
	// Convert libsql:// to https://
	if len(url) > 9 && url[:9] == "libsql://" {
		url = "https://" + url[9:]
	}
	return &TursoClient{
		URL:   url,
		Token: token,
		HTTP:  &http.Client{Timeout: 30 * time.Second},
	}
}

type TursoArg struct {
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

type TursoStmt struct {
	SQL  string      `json:"sql"`
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
	Columns []TursoColumn   `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
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
	body := TursoPipelineBody{
		Requests: []TursoRequest{{Type: "execute", Stmt: TursoStmt{SQL: sql, Args: args}}},
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", c.URL+"/v2/pipeline", bytes.NewReader(b))
	if err != nil {
		return TursoResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return TursoResult{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var tr TursoResponse
	if err := json.Unmarshal(raw, &tr); err != nil {
		return TursoResult{}, fmt.Errorf("parse: %w (%s)", err, string(raw))
	}
	if len(tr.Results) == 0 {
		return TursoResult{}, fmt.Errorf("no results: %s", string(raw))
	}
	if tr.Results[0].Error != nil {
		return TursoResult{}, fmt.Errorf("sql error: %s", tr.Results[0].Error.Message)
	}
	return tr.Results[0].Response.Result, nil
}

func Text(v string) TursoArg { return TursoArg{Type: "text", Value: v} }
func Int(v int64) TursoArg   { return TursoArg{Type: "integer", Value: v} }
func Null() TursoArg         { return TursoArg{Type: "null", Value: nil} }