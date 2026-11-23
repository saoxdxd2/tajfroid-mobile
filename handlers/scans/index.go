package handler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	db := lib.NewTurso()
	switch r.Method {
	case http.MethodPost:
		var body struct {
			ClientID      string `json:"clientId"`
			CapturedAt    int64  `json:"capturedAt"`
			SizeBytes     int64  `json:"sizeBytes"`
			Sha256        string `json:"sha256"`
			CiphertextB64 string `json:"ciphertextB64"`
			ContentType   string `json:"contentType"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if body.ClientID == "" {
			http.Error(w, "clientId required", http.StatusBadRequest)
			return
		}
		ident := requestIdentity(r)
		deviceID := ident.ID
		if ident.Kind == "device" && deviceID != "" {
			// ok
		} else {
			// Agent or unknown caller: attribute to a synthetic device row so the
			// scans.device_id FK is satisfied.
			deviceID = "desktop-agent"
			now := time.Now().Unix()
			db.Execute(`INSERT INTO devices (id, device_token_hash, name, paired_at, created_at)
				VALUES (?,?,?,?,?) ON CONFLICT(id) DO NOTHING`,
				lib.Text(deviceID), lib.Text("agent"), lib.Text("Desktop agent"), lib.Int(now), lib.Int(now))
		}
		res, _ := db.Execute("SELECT id, status FROM scans WHERE client_id = ? LIMIT 1", lib.Text(body.ClientID))
		if len(res.Rows) > 0 {
			// Idempotent retry: if client now has ciphertext and we don't, attach it.
			scanID := lib.ToString(res.Rows[0][0])
			if body.CiphertextB64 != "" {
				db.Execute("UPDATE scans SET ciphertext_b64=?, sha256=COALESCE(?,sha256), size_bytes=?, content_type=?, updated_at=? WHERE id=? AND (ciphertext_b64 IS NULL OR ciphertext_b64='')",
					lib.Text(body.CiphertextB64), nullableText(body.Sha256), lib.Int(body.SizeBytes),
					nullableText(body.ContentType), lib.Int(time.Now().Unix()), lib.Text(scanID))
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"scanId": scanID, "status": res.Rows[0][1], "idempotent": true,
			})
			return
		}
		if body.CiphertextB64 != "" {
			// Basic sanity: must be valid base64 and within size budget.
			raw, err := base64.StdEncoding.DecodeString(body.CiphertextB64)
			if err != nil {
				http.Error(w, "ciphertextB64 is not valid base64", http.StatusBadRequest)
				return
			}
			if len(raw) > 4*1024*1024 {
				http.Error(w, "encrypted scan too large", http.StatusRequestEntityTooLarge)
				return
			}
			if body.SizeBytes == 0 {
				body.SizeBytes = int64(len(raw))
			}
		}
		id := lib.NewID()
		now := time.Now().Unix()
		if body.CapturedAt == 0 {
			body.CapturedAt = now
		}
		status := "uploaded"
		if body.CiphertextB64 == "" {
			status = "pending_upload"
		}
		_, err := db.Execute(`INSERT INTO scans
			(id, client_id, device_id, captured_at, status, blob_key, size_bytes, sha256, ciphertext_b64, content_type, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			lib.Text(id), lib.Text(body.ClientID), lib.Text(deviceID), lib.Int(body.CapturedAt),
			lib.Text(status), lib.Null(), lib.Int(body.SizeBytes), nullableText(body.Sha256),
			nullableText(body.CiphertextB64), nullableText(body.ContentType),
			lib.Int(now), lib.Int(now))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		db.Execute("INSERT INTO scan_events (scan_id, to_status, at) VALUES (?,?,?)",
			lib.Text(id), lib.Text(status), lib.Int(now))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"scanId": id, "status": status})

	case http.MethodGet:
		since := int64(0)
		if v := r.URL.Query().Get("since"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				since = n
			}
		}
		ident := requestIdentity(r)
		q := `SELECT id, client_id, status, purchase_reference, message, updated_at, size_bytes
			FROM scans WHERE updated_at > ?`
		args := []lib.TursoArg{lib.Int(since)}
		if ident.Kind == "device" && ident.ID != "" {
			q += " AND device_id = ?"
			args = append(args, lib.Text(ident.ID))
		}
		q += " ORDER BY updated_at ASC LIMIT 200"
		res, err := db.Execute(q, args...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		scans := []map[string]interface{}{}
		cursor := since
		for _, row := range res.Rows {
			updatedAt := lib.ToInt64(row[5])
			scans = append(scans, map[string]interface{}{
				"id": row[0], "clientId": row[1], "status": row[2],
				"purchaseReference": row[3], "message": row[4], "updatedAt": updatedAt,
				"sizeBytes": row[6],
			})
			if updatedAt > cursor {
				cursor = updatedAt
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"scans": scans, "cursor": cursor})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// requestIdentity uses the router's single auth lookup. The fallback keeps
// direct handler calls (tests/local tools) working without a router wrapper.
func requestIdentity(r *http.Request) lib.Identity {
	if id, ok := lib.IdentityFrom(r); ok {
		return id
	}
	id, _ := lib.Authenticate(r)
	return id
}

func nullableText(s string) lib.TursoArg {
	if s == "" {
		return lib.Null()
	}
	return lib.Text(s)
}
