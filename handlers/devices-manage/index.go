package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	db := lib.NewTurso()

	switch r.Method {
	case http.MethodGet:
		res, err := db.Execute(`SELECT id, name, paired_at, revoked_at, created_at FROM devices WHERE id=?`, lib.Text(id))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(res.Rows) == 0 {
			http.Error(w, "device not found", http.StatusNotFound)
			return
		}
		row := res.Rows[0]
		lib.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"id": row[0], "name": row[1], "pairedAt": row[2],
			"revokedAt": row[3], "baseRevokedAt": lib.ToInt64(row[3]),
			"createdAt": row[4],
		})

	case http.MethodDelete:
		var body struct {
			BaseRevokedAt int64 `json:"baseRevokedAt"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		cur, err := db.Execute("SELECT revoked_at FROM devices WHERE id=?", lib.Text(id))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(cur.Rows) == 0 {
			http.Error(w, "device not found", http.StatusNotFound)
			return
		}
		current := lib.ToInt64(cur.Rows[0][0])
		// revoked_at doubles as the device's optimistic-concurrency token:
		// 0 means active, any positive value means revoked at that timestamp.
		if current > 0 && body.BaseRevokedAt != current {
			writeConflict(w, id, current)
			return
		}
		if current == 0 && body.BaseRevokedAt > 0 {
			// The client thinks this device was revoked, but the server sees it
			// active again (e.g. re-pairing reset the row). Never revoke blindly.
			writeConflict(w, id, current)
			return
		}
		if current > 0 {
			lib.WriteJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "revoked": false})
			return
		}

		res, err := db.Execute("UPDATE devices SET revoked_at=? WHERE id=? AND revoked_at IS NULL",
			lib.Int(time.Now().Unix()), lib.Text(id))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if res.AffectedRowCount == 0 {
			// Lost a race between the SELECT and UPDATE: re-read and fail with
			// the current token instead of silently succeeding.
			again, e := db.Execute("SELECT revoked_at FROM devices WHERE id=?", lib.Text(id))
			if e == nil && len(again.Rows) > 0 {
				current = lib.ToInt64(again.Rows[0][0])
			}
			writeConflict(w, id, current)
			return
		}
		lib.WriteJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "revoked": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeConflict(w http.ResponseWriter, id string, current int64) {
	lib.WriteJSON(w, http.StatusConflict, map[string]interface{}{
		"error":            "conflict: device changed on another device",
		"currentUpdatedAt": current,
		"currentRevokedAt": current,
		"record": map[string]interface{}{
			"id": id, "revokedAt": current, "baseRevokedAt": current,
		},
	})
}
