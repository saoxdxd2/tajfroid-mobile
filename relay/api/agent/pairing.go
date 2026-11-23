package main

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"time"

	"tajfroid-relay/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	code := randomCode()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"pairingCode":      code,
		"pairingSessionId": lib.NewID(),
		"expiresAt":        time.Now().Unix() + 300,
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