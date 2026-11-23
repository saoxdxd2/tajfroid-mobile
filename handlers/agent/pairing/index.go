package handler

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"time"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	db := lib.NewTurso()
	code := randomCode()
	sessionID := lib.NewID()
	now := time.Now().Unix()
	expiresAt := now + 600
	if _, err := db.Execute(
		"INSERT INTO pairing_sessions (id, code, expires_at, created_at) VALUES (?,?,?,?)",
		lib.Text(sessionID), lib.Text(code), lib.Int(expiresAt), lib.Int(now)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Expired sessions have no value; drop them opportunistically so the table
	// stays tiny on the free tier.
	db.Execute("DELETE FROM pairing_sessions WHERE expires_at < ?", lib.Int(now-86400))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"pairingCode":      code,
		"pairingSessionId": sessionID,
		"expiresAt":        expiresAt,
	})
}

func randomCode() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}
