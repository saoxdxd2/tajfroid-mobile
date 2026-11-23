package handler

import (
	"encoding/base64"
	"net/http"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	db := lib.NewTurso()
	res, err := db.Execute("SELECT ciphertext_b64, content_type, size_bytes FROM scans WHERE id = ?", lib.Text(id))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(res.Rows) == 0 {
		http.Error(w, "scan not found", http.StatusNotFound)
		return
	}
	b64 := lib.ToString(res.Rows[0][0])
	if b64 == "" {
		http.Error(w, "scan file not uploaded yet", http.StatusNotFound)
		return
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		http.Error(w, "stored scan is corrupt", http.StatusInternalServerError)
		return
	}
	ct := lib.ToString(res.Rows[0][1])
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconvItoa(len(raw)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func strconvItoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
