package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	db := lib.NewTurso()
	switch r.Method {
	case http.MethodGet:
		includeArchived := r.URL.Query().Get("archived") == "1"
		q := "SELECT id, party_type, company_name, first_name, last_name, contact_name, email, phone, ice_number, is_archived, created_at, updated_at FROM customers"
		if !includeArchived {
			q += " WHERE is_archived = 0"
		}
		q += " ORDER BY company_name ASC"
		res, err := db.Execute(q)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items := []map[string]interface{}{}
		for _, row := range res.Rows {
			items = append(items, map[string]interface{}{
				"id": row[0], "partyType": row[1], "companyName": row[2],
				"firstName": nullableStr(row[3]), "lastName": nullableStr(row[4]),
				"contactName": nullableStr(row[5]), "email": nullableStr(row[6]),
				"phone": nullableStr(row[7]), "iceNumber": nullableStr(row[8]),
				"isArchived": lib.ToInt(row[9]) == 1, "createdAt": row[10], "updatedAt": row[11],
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"customers": items})

	case http.MethodPost:
		var body struct {
			ID          string `json:"id"` // optional client idempotency key
			PartyType   string `json:"partyType"`
			CompanyName string `json:"companyName"`
			FirstName   string `json:"firstName"`
			LastName    string `json:"lastName"`
			ContactName string `json:"contactName"`
			Email       string `json:"email"`
			Phone       string `json:"phone"`
			IceNumber   string `json:"iceNumber"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.ID != "" {
			if ex, err := db.Execute("SELECT id, company_name FROM customers WHERE id=?", lib.Text(body.ID)); err == nil && len(ex.Rows) > 0 {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{"id": body.ID, "companyName": ex.Rows[0][1], "idempotent": true})
				return
			}
		}
		if body.CompanyName == "" && body.FirstName == "" {
			http.Error(w, "companyName or firstName required", http.StatusBadRequest)
			return
		}
		if body.PartyType == "" {
			body.PartyType = "enterprise"
		}
		// For individuals, derive company_name from first/last name
		companyName := body.CompanyName
		if body.PartyType == "individual" {
			companyName = body.FirstName
			if body.LastName != "" {
				companyName += " " + body.LastName
			}
		}
		now := time.Now().Unix()
		id := body.ID
		if id == "" {
			id = lib.NewID()
		}
		_, err := db.Execute(`INSERT INTO customers (id, party_type, company_name, first_name, last_name, contact_name, email, phone, ice_number, is_archived, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			lib.Text(id), lib.Text(body.PartyType), lib.Text(companyName),
			nullableText(body.FirstName), nullableText(body.LastName),
			nullableText(body.ContactName), nullableText(body.Email),
			nullableText(body.Phone), nullableText(body.IceNumber),
			lib.Int(0), lib.Int(now), lib.Int(now))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": id, "companyName": companyName})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func nullableStr(v interface{}) string {
	if v == nil {
		return ""
	}
	return lib.ToString(v)
}

func nullableText(s string) lib.TursoArg {
	if s == "" {
		return lib.Null()
	}
	return lib.Text(s)
}