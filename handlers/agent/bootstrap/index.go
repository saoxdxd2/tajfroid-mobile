package handler

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"tajfroid-mobile/lib"
)

// Bootstrap issues the desktop agent token. It is inherently unauthenticated
// (the agent has no token yet), so it is limited to the very first call: once
// one agent token exists, only an authenticated agent may mint another one.
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		DeviceID string `json:"deviceId"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.DeviceID == "" {
		http.Error(w, "deviceId required", http.StatusBadRequest)
		return
	}
	db := lib.NewTurso()
	count, err := db.Execute("SELECT id FROM agent_tokens LIMIT 1")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(count.Rows) > 0 {
		authorized := false
		if key := os.Getenv("BOOTSTRAP_RECOVERY_KEY"); key != "" {
			if got := r.Header.Get("X-Bootstrap-Recovery"); got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(key)) == 1 {
				authorized = true
			}
		}
		if !authorized {
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				tok := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
				if tok != "" {
					h := lib.Sha256Hex(tok)
					if row, err := db.Execute("SELECT id FROM agent_tokens WHERE token_hash=?", lib.Text(h)); err == nil && len(row.Rows) > 0 {
						authorized = true
					}
				}
			}
		}
		if !authorized {
			http.Error(w, "an agent is already registered; provide valid agent Authorization or X-Bootstrap-Recovery", http.StatusConflict)
			return
		}
	}
	token := lib.NewToken()
	hash := lib.Sha256Hex(token)
	if len(count.Rows) > 0 {
		// Recovery/re-bootstrap: replace old agent tokens to avoid orphaned valid credentials.
		if _, err := db.Execute("DELETE FROM agent_tokens"); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if _, err := db.Execute("INSERT INTO agent_tokens (id, token_hash, device_id, created_at) VALUES (?,?,?,?)",
		lib.Text(lib.NewID()), lib.Text(hash), lib.Text(body.DeviceID), lib.Int(time.Now().Unix())); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"agentToken": token})
}
