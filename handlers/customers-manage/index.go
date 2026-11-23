package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	db := lib.NewTurso()
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		res, err := db.Execute("SELECT id, party_type, company_name, first_name, last_name, contact_name, email, phone, ice_number, is_archived, updated_at FROM customers WHERE id = ?", lib.Text(id))
		if err != nil || len(res.Rows) == 0 {
			http.Error(w, "customer not found", http.StatusNotFound)
			return
		}
		row := res.Rows[0]
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": row[0], "partyType": row[1], "companyName": row[2],
			"firstName": lib.NullStr(row[3]), "lastName": lib.NullStr(row[4]),
			"contactName": lib.NullStr(row[5]), "email": lib.NullStr(row[6]),
			"phone": lib.NullStr(row[7]), "iceNumber": lib.NullStr(row[8]),
			"isArchived": lib.ToInt(row[9]) == 1, "updatedAt": lib.ToInt64(row[10]),
		})

	case http.MethodPut:
		var body struct {
			PartyType     string `json:"partyType"`
			CompanyName   string `json:"companyName"`
			FirstName     string `json:"firstName"`
			LastName      string `json:"lastName"`
			ContactName   string `json:"contactName"`
			Email         string `json:"email"`
			Phone         string `json:"phone"`
			IceNumber     string `json:"iceNumber"`
			BaseUpdatedAt int64  `json:"baseUpdatedAt"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		// Optimistic concurrency: if client sent the version it edited, reject
		// stale writes so the user can merge instead of silently overwriting.
		if body.BaseUpdatedAt > 0 {
			cur, err := db.Execute("SELECT updated_at FROM customers WHERE id=?", lib.Text(id))
			if err == nil && len(cur.Rows) > 0 {
				if lib.ToInt64(cur.Rows[0][0]) != body.BaseUpdatedAt {
					lib.WriteConflict(w, lib.ToInt64(cur.Rows[0][0]))
					return
				}
			}
		}
		now := time.Now().Unix()
		if body.CompanyName != "" {
			db.Execute("UPDATE customers SET company_name=? WHERE id=?", lib.Text(body.CompanyName), lib.Text(id))
		}
		if body.PartyType != "" {
			db.Execute("UPDATE customers SET party_type=? WHERE id=?", lib.Text(body.PartyType), lib.Text(id))
		}
		if body.FirstName != "" {
			db.Execute("UPDATE customers SET first_name=? WHERE id=?", lib.Text(body.FirstName), lib.Text(id))
		}
		if body.LastName != "" {
			db.Execute("UPDATE customers SET last_name=? WHERE id=?", lib.Text(body.LastName), lib.Text(id))
		}
		if body.ContactName != "" {
			db.Execute("UPDATE customers SET contact_name=? WHERE id=?", lib.Text(body.ContactName), lib.Text(id))
		}
		if body.Email != "" {
			db.Execute("UPDATE customers SET email=? WHERE id=?", lib.Text(body.Email), lib.Text(id))
		}
		if body.Phone != "" {
			db.Execute("UPDATE customers SET phone=? WHERE id=?", lib.Text(body.Phone), lib.Text(id))
		}
		if body.IceNumber != "" {
			db.Execute("UPDATE customers SET ice_number=? WHERE id=?", lib.Text(body.IceNumber), lib.Text(id))
		}
		db.Execute("UPDATE customers SET updated_at=? WHERE id=?", lib.Int(now), lib.Text(id))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

	case http.MethodDelete:
		if v := r.URL.Query().Get("baseUpdatedAt"); v != "" {
			if base, err := strconv.ParseInt(v, 10, 64); err == nil && base > 0 {
				if cur, err := db.Execute("SELECT updated_at FROM customers WHERE id=?", lib.Text(id)); err == nil && len(cur.Rows) > 0 && lib.ToInt64(cur.Rows[0][0]) != base {
					lib.WriteConflict(w, lib.ToInt64(cur.Rows[0][0]))
					return
				}
			}
		}
		now := time.Now().Unix()
		db.Execute("UPDATE customers SET is_archived=1, updated_at=? WHERE id=?", lib.Int(now), lib.Text(id))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

