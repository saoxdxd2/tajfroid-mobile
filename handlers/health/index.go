package handler

import (
	"net/http"
	"time"

	"tajfroid-mobile/lib"
)

const (
	deviceTokenIndex = "idx_devices_token_hash"
	agentTokenIndex  = "idx_agent_tokens_token_hash"
)

// healthResponse keeps the legacy "ok" field while adding the richer status,
// check and latency shape used by production verification.
type healthResponse struct {
	OK        bool            `json:"ok"`
	Status    string          `json:"status"`
	TS        int64           `json:"ts"`
	LatencyMS float64         `json:"latency_ms"`
	Checks    map[string]bool `json:"checks"`
	Indexes   map[string]bool `json:"indexes"`
	Error     string          `json:"error,omitempty"`
}

func newHealthResponse(status string, latencyMS float64) healthResponse {
	return healthResponse{
		OK:        status == "ok",
		Status:    status,
		TS:        time.Now().Unix(),
		LatencyMS: latencyMS,
		Checks: map[string]bool{
			"database":     false,
			"auth_indexes": false,
		},
		Indexes: map[string]bool{
			deviceTokenIndex: false,
			agentTokenIndex:  false,
		},
	}
}

func writeHealth(w http.ResponseWriter, status int, resp healthResponse) {
	lib.WriteJSON(w, status, resp)
}

func Handler(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	db := lib.NewTurso()
	if db.URL == "" || db.Token == "" {
		resp := newHealthResponse("degraded", lib.DurationMS(time.Since(started)))
		resp.Error = "TURSO_DATABASE_URL and TURSO_AUTH_TOKEN must be set"
		writeHealth(w, http.StatusServiceUnavailable, resp)
		return
	}

	// One round-trip for liveness and for the two auth indexes every request
	// depends on. Raw errors are never echoed (they can carry environment
	// details); only a stable category is returned.
	results, err := db.ExecuteBatch(r.Context(),
		lib.TursoStmt{SQL: "SELECT 1"},
		lib.TursoStmt{
			SQL:  `SELECT name FROM sqlite_master WHERE type = 'index' AND name IN (?, ?)`,
			Args: []lib.TursoArg{lib.Text(deviceTokenIndex), lib.Text(agentTokenIndex)},
		},
	)
	latencyMS := lib.DurationMS(time.Since(started))
	if err != nil {
		resp := newHealthResponse("degraded", latencyMS)
		resp.Error = "database check failed"
		writeHealth(w, http.StatusServiceUnavailable, resp)
		return
	}
	if len(results) != 2 {
		resp := newHealthResponse("degraded", latencyMS)
		resp.Error = "unexpected database response"
		writeHealth(w, http.StatusServiceUnavailable, resp)
		return
	}

	resp := newHealthResponse("ok", latencyMS)
	resp.Checks["database"] = true
	for _, row := range results[1].Rows {
		if len(row) == 0 {
			continue
		}
		name := lib.ToString(row[0])
		if _, known := resp.Indexes[name]; known {
			resp.Indexes[name] = true
		}
	}
	if resp.Indexes[deviceTokenIndex] && resp.Indexes[agentTokenIndex] {
		resp.Checks["auth_indexes"] = true
	} else {
		resp.Status = "degraded"
		resp.OK = false
		resp.Error = "auth indexes missing"
	}

	status := http.StatusOK
	if resp.Status != "ok" {
		status = http.StatusServiceUnavailable
	}
	writeHealth(w, status, resp)
}
