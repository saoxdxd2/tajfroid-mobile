package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		PairingCode string `json:"pairingCode"`
		DeviceName  string `json:"deviceName"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.PairingCode == "" {
		http.Error(w, "pairingCode required", http.StatusBadRequest)
		return
	}
	db := lib.NewTurso()
	deviceID := lib.NewID()
	token := lib.NewToken()
	hash := lib.Sha256Hex(token)
	now := time.Now().Unix()
	db.Execute("INSERT INTO devices (id, device_token_hash, name, paired_at, created_at) VALUES (?,?,?,?,?)",
		lib.Text(deviceID), lib.Text(hash), lib.Text(body.DeviceName), lib.Int(now), lib.Int(now))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"deviceId": deviceID, "deviceToken": token})
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func newToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

