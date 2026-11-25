package handler

import (
	"encoding/json"
	"net/http"
	"strings"
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
	code := strings.ToUpper(strings.TrimSpace(body.PairingCode))
	if code == "" {
		http.Error(w, "pairingCode required", http.StatusBadRequest)
		return
	}
	db := lib.NewTurso()
	now := time.Now().Unix()

	// A pairing code must have been issued by the desktop agent, must not be
	// expired and must never have been used before. This is what stops a
	// stranger who finds the relay from minting a device token.
	res, err := db.Execute(
		"SELECT id, expires_at, used_at FROM pairing_sessions WHERE code = ? ORDER BY created_at DESC LIMIT 1",
		lib.Text(code))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(res.Rows) == 0 {
		http.Error(w, "invalid pairing code", http.StatusUnauthorized)
		return
	}
	sessionID := lib.ToString(res.Rows[0][0])
	expiresAt := lib.ToInt64(res.Rows[0][1])
	if res.Rows[0][2] != nil || expiresAt < now {
		http.Error(w, "pairing code expired; generate a new QR code on the desktop", http.StatusUnauthorized)
		return
	}

	deviceID := lib.NewID()
	token := lib.NewToken()
	hash := lib.Sha256Hex(token)
	name := strings.TrimSpace(body.DeviceName)
	if name == "" {
		name = "Phone"
	}
	// Consume the code atomically: the UPDATE only matches an unused session,
	// so two phones racing on the same QR code cannot both win.
	consumed, err := db.Execute(
		"UPDATE pairing_sessions SET used_at = ?, device_id = ? WHERE id = ? AND used_at IS NULL",
		lib.Int(now), lib.Text(deviceID), lib.Text(sessionID))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if consumed.AffectedRowCount == 0 {
		http.Error(w, "pairing code was already used; generate a new QR code", http.StatusUnauthorized)
		return
	}
	if _, err := db.Execute(
		"INSERT INTO devices (id, device_token_hash, name, paired_at, created_at) VALUES (?,?,?,?,?)",
		lib.Text(deviceID), lib.Text(hash), lib.Text(name), lib.Int(now), lib.Int(now)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     lib.DeviceCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   60 * 60 * 24 * 365,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"deviceId": deviceID, "deviceToken": token})
}
