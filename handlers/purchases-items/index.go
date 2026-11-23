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
		purchaseID := r.URL.Query().Get("purchaseId")
		if purchaseID == "" {
			http.Error(w, "purchaseId required", http.StatusBadRequest)
			return
		}
		items, err := listItems(db, purchaseID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"items": items})

	case http.MethodPost:
		var body struct {
			ID          string `json:"id"` // optional client idempotency key
			PurchaseID  string `json:"purchaseId"`
			ProductSKU  string `json:"productSku"`
			ProductName string `json:"productName"`
			Quantity    int    `json:"quantity"`
			UnitCost    string `json:"unitCost"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.ID != "" {
			if ex, err := db.Execute("SELECT id FROM purchase_items WHERE id=?", lib.Text(body.ID)); err == nil && len(ex.Rows) > 0 {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{"id": body.ID, "idempotent": true})
				return
			}
		}
		if body.PurchaseID == "" || body.ProductSKU == "" {
			http.Error(w, "purchaseId and productSku required", http.StatusBadRequest)
			return
		}
		if body.Quantity <= 0 {
			http.Error(w, "quantity must be positive", http.StatusBadRequest)
			return
		}
		state, err := getPurchaseState(db, body.PurchaseID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if state != "Draft" {
			http.Error(w, "cannot add items to a purchase that is not in Draft state", http.StatusBadRequest)
			return
		}
		prod, err := db.Execute("SELECT name, purchase_price FROM catalog WHERE sku = ?", lib.Text(body.ProductSKU))
		if err != nil || len(prod.Rows) == 0 {
			http.Error(w, "product not found", http.StatusNotFound)
			return
		}
		name := body.ProductName
		if name == "" {
			name = lib.ToString(prod.Rows[0][0])
		}
		cost := body.UnitCost
		if cost == "" {
			cost = lib.ToString(prod.Rows[0][1])
		}
		if lib.ParseMoney(cost) < 0 {
			http.Error(w, "unit cost cannot be negative", http.StatusBadRequest)
			return
		}
		id := body.ID
		if id == "" {
			id = lib.NewID()
		}
		now := time.Now().Unix()
		_, err = db.Execute(`INSERT INTO purchase_items
			(id, purchase_id, product_sku, product_name, quantity, unit_cost, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?)`,
			lib.Text(id), lib.Text(body.PurchaseID), lib.Text(body.ProductSKU), lib.Text(name),
			lib.Int(int64(body.Quantity)), lib.Text(lib.FormatMoney(lib.ParseMoney(cost))),
			lib.Int(now), lib.Int(now))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		total, err := recalcTotal(db, body.PurchaseID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{"id": id, "totalAmount": lib.FormatMoney(total)})

	case http.MethodDelete:
		itemID := r.URL.Query().Get("id")
		if itemID == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		res, err := db.Execute("SELECT purchase_id FROM purchase_items WHERE id = ?", lib.Text(itemID))
		if err != nil || len(res.Rows) == 0 {
			http.Error(w, "item not found", http.StatusNotFound)
			return
		}
		purchaseID := lib.ToString(res.Rows[0][0])
		state, err := getPurchaseState(db, purchaseID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if state != "Draft" {
			http.Error(w, "cannot modify a purchase that is not in Draft state", http.StatusBadRequest)
			return
		}
		if _, err := db.Execute("DELETE FROM purchase_items WHERE id = ?", lib.Text(itemID)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		total, err := recalcTotal(db, purchaseID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "totalAmount": lib.FormatMoney(total)})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func getPurchaseState(db *lib.TursoClient, id string) (string, error) {
	res, err := db.Execute("SELECT state FROM purchases WHERE id = ?", lib.Text(id))
	if err != nil {
		return "", err
	}
	if len(res.Rows) == 0 {
		return "", errText("purchase not found")
	}
	return lib.ToString(res.Rows[0][0]), nil
}

func listItems(db *lib.TursoClient, purchaseID string) ([]map[string]interface{}, error) {
	res, err := db.Execute(`SELECT id, product_sku, product_name, quantity, unit_cost, created_at
		FROM purchase_items WHERE purchase_id = ? ORDER BY created_at ASC`, lib.Text(purchaseID))
	if err != nil {
		return nil, err
	}
	items := []map[string]interface{}{}
	for _, row := range res.Rows {
		items = append(items, map[string]interface{}{
			"id": row[0], "productSku": row[1], "productName": nullableStr(row[2]),
			"quantity": lib.ToInt(row[3]), "unitCost": row[4], "createdAt": row[5],
		})
	}
	return items, nil
}

func recalcTotal(db *lib.TursoClient, purchaseID string) (int64, error) {
	res, err := db.Execute("SELECT quantity, unit_cost FROM purchase_items WHERE purchase_id = ?", lib.Text(purchaseID))
	if err != nil {
		return 0, err
	}
	total := int64(0)
	for _, row := range res.Rows {
		total += int64(lib.ToInt(row[0])) * lib.ParseMoney(lib.ToString(row[1]))
	}
	now := time.Now().Unix()
	_, err = db.Execute("UPDATE purchases SET total_amount=?, updated_at=? WHERE id=?",
		lib.Text(lib.FormatMoney(total)), lib.Int(now), lib.Text(purchaseID))
	return total, err
}

func nullableStr(v interface{}) string {
	if v == nil {
		return ""
	}
	return lib.ToString(v)
}

type errText string

func (e errText) Error() string { return string(e) }
