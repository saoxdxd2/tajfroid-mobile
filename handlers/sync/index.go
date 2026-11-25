package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tajfroid-mobile/lib"
)

type syncOpInput struct {
	OpID      string      `json:"opId"`
	DeviceID  string      `json:"deviceId"`
	Entity    string      `json:"entity"`
	EntityID  string      `json:"entityId"`
	OpKind    string      `json:"opKind"` // "field_set" | "stock_delta"
	Field     string      `json:"field"`
	Value     interface{} `json:"value"`
	DeltaInt  int64       `json:"deltaInt"`
	HLC       string      `json:"hlc"`
	BaseHLC   string      `json:"baseHlc"`
	ForceWin  bool        `json:"forceWin"`
}

var allowedCatalogFields = map[string]string{
	"name":         "name",
	"description":  "description",
	"sale_price":   "sale_price",
	"pinned_price": "pinned_price",
	"vat_rate":     "vat_rate",
	"category":     "category",
	"brand":        "brand",
	"sku":          "sku",
	"display_sku":  "display_sku",
	"no_sku":       "no_sku",
	"sku_warning":  "sku_warning",
	"is_return":    "is_return",
	"return_qty":   "return_qty",
	"sealed_qty":   "sealed_qty",
	"return_label": "return_label",
	"price_status": "price_status",
	"active":       "active",
}

func Handler(w http.ResponseWriter, r *http.Request) {
	db := lib.NewTurso()
	switch r.Method {
	case http.MethodGet:
		sinceSeq := int64(0)
		if raw := r.URL.Query().Get("since_seq"); raw != "" {
			v, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || v < 0 {
				http.Error(w, "invalid since_seq", http.StatusBadRequest)
				return
			}
			sinceSeq = v
		}
		res, err := db.ExecuteContext(r.Context(),
			`SELECT seq, op_id, device_id, entity, entity_id, op_kind, field, value_json, delta_int, hlc, base_hlc, created_at
			 FROM sync_ops WHERE seq > ? ORDER BY seq ASC LIMIT 500`,
			lib.Int(sinceSeq))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ops := make([]map[string]interface{}, 0, len(res.Rows))
		maxSeq := sinceSeq
		for _, row := range res.Rows {
			seq := lib.ToInt64(row[0])
			if seq > maxSeq {
				maxSeq = seq
			}
			ops = append(ops, map[string]interface{}{
				"seq":       seq,
				"opId":      lib.ToString(row[1]),
				"deviceId":  lib.ToString(row[2]),
				"entity":    lib.ToString(row[3]),
				"entityId":  lib.ToString(row[4]),
				"opKind":    lib.ToString(row[5]),
				"field":     lib.NullStr(row[6]),
				"valueJson": lib.NullStr(row[7]),
				"deltaInt":  lib.ToInt64(row[8]),
				"hlc":       lib.ToString(row[9]),
				"baseHlc":   lib.NullStr(row[10]),
				"createdAt": lib.ToInt64(row[11]),
			})
		}
		lib.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"ok":     true,
			"maxSeq": maxSeq,
			"ops":    ops,
		})

	case http.MethodPost:
		var body struct {
			Ops []syncOpInput `json:"ops"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid sync payload", http.StatusBadRequest)
			return
		}
		if len(body.Ops) > 250 {
			http.Error(w, "too many ops in one batch (max 250)", http.StatusBadRequest)
			return
		}
		identity, _ := lib.IdentityFrom(r)
		now := time.Now().Unix()

		applied := make([]string, 0)
		duplicates := make([]string, 0)
		stale := make([]string, 0)
		conflicts := make([]map[string]interface{}, 0)

		for _, op := range body.Ops {
			opID := strings.TrimSpace(op.OpID)
			entity := strings.TrimSpace(op.Entity)
			entityID := strings.TrimSpace(op.EntityID)
			deviceID := strings.TrimSpace(op.DeviceID)
			if deviceID == "" {
				deviceID = identity.ID
			}
			if opID == "" || entity == "" || entityID == "" {
				http.Error(w, "opId, entity, and entityId required", http.StatusBadRequest)
				return
			}
			if _, err := lib.ParseHLC(op.HLC); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			// Idempotency guard by op_id
			seen, err := db.ExecuteContext(r.Context(), "SELECT seq FROM sync_ops WHERE op_id = ? LIMIT 1", lib.Text(opID))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if len(seen.Rows) > 0 {
				duplicates = append(duplicates, opID)
				continue
			}

			valBytes, _ := json.Marshal(op.Value)
			valJSON := string(valBytes)

			if op.OpKind == "stock_delta" && entity == "catalog" {
				stmts := []lib.TursoStmt{
					{
						SQL: `INSERT INTO sync_ops (op_id, device_id, entity, entity_id, op_kind, field, value_json, delta_int, hlc, base_hlc, created_at)
						      VALUES (?, ?, ?, ?, 'stock_delta', 'stock', ?, ?, ?, ?, ?)`,
						Args: []lib.TursoArg{lib.Text(opID), lib.Text(deviceID), lib.Text(entity), lib.Text(entityID), lib.Text(valJSON), lib.Int(op.DeltaInt), lib.Text(op.HLC), lib.Text(op.BaseHLC), lib.Int(now)},
					},
					{
						SQL:  `UPDATE catalog SET stock = MAX(0, stock + ?), sealed_qty = MAX(0, sealed_qty + ?) WHERE sku = ?`,
						Args: []lib.TursoArg{lib.Int(op.DeltaInt), lib.Int(op.DeltaInt), lib.Text(entityID)},
					},
					{
						SQL:  lib.CatalogMetaBumpSQL,
						Args: []lib.TursoArg{lib.Int(now), lib.Int(now)},
					},
				}
				if _, err := db.ExecuteBatch(r.Context(), stmts...); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				applied = append(applied, opID)
				continue
			}

			if op.OpKind == "field_set" {
				curRes, err := db.ExecuteContext(r.Context(),
					`SELECT hlc, device_id, value_json FROM sync_field_clocks WHERE entity = ? AND entity_id = ? AND field = ? LIMIT 1`,
					lib.Text(entity), lib.Text(entityID), lib.Text(op.Field))
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				var curHLC, curDev, curVal string
				if len(curRes.Rows) > 0 {
					curHLC = lib.ToString(curRes.Rows[0][0])
					curDev = lib.ToString(curRes.Rows[0][1])
					curVal = lib.NullStr(curRes.Rows[0][2])
				}
				decision, err := lib.FieldConflictDecision(curHLC, op.BaseHLC, op.HLC, op.ForceWin)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				if decision == "stale" {
					stale = append(stale, opID)
					continue
				}
				if decision == "conflict" {
					conflicts = append(conflicts, map[string]interface{}{
						"opId":          opID,
						"entity":        entity,
						"entityId":      entityID,
						"field":         op.Field,
						"serverHlc":     curHLC,
						"serverDevice":  curDev,
						"serverValue":   curVal,
						"incomingHlc":   op.HLC,
						"incomingValue": op.Value,
					})
					continue
				}

				stmts := []lib.TursoStmt{
					{
						SQL: `INSERT INTO sync_ops (op_id, device_id, entity, entity_id, op_kind, field, value_json, delta_int, hlc, base_hlc, created_at)
						      VALUES (?, ?, ?, ?, 'field_set', ?, ?, 0, ?, ?, ?)`,
						Args: []lib.TursoArg{lib.Text(opID), lib.Text(deviceID), lib.Text(entity), lib.Text(entityID), lib.Text(op.Field), lib.Text(valJSON), lib.Text(op.HLC), lib.Text(op.BaseHLC), lib.Int(now)},
					},
					{
						SQL: `INSERT INTO sync_field_clocks (entity, entity_id, field, hlc, device_id, value_json, updated_at)
						      VALUES (?, ?, ?, ?, ?, ?, ?)
						      ON CONFLICT(entity, entity_id, field) DO UPDATE SET hlc=excluded.hlc, device_id=excluded.device_id, value_json=excluded.value_json, updated_at=excluded.updated_at`,
						Args: []lib.TursoArg{lib.Text(entity), lib.Text(entityID), lib.Text(op.Field), lib.Text(op.HLC), lib.Text(deviceID), lib.Text(valJSON), lib.Int(now)},
					},
				}

				if entity == "catalog" {
					if col, ok := allowedCatalogFields[op.Field]; ok {
						arg := toTursoArg(op.Value)
						stmts = append(stmts, lib.TursoStmt{
							SQL:  "UPDATE catalog SET " + col + " = ? WHERE sku = ?",
							Args: []lib.TursoArg{arg, lib.Text(entityID)},
						})
					}
					stmts = append(stmts, lib.TursoStmt{
						SQL:  lib.CatalogMetaBumpSQL,
						Args: []lib.TursoArg{lib.Int(now), lib.Int(now)},
					})
				}

				if _, err := db.ExecuteBatch(r.Context(), stmts...); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				applied = append(applied, opID)
				continue
			}

			http.Error(w, "unsupported opKind or entity", http.StatusBadRequest)
			return
		}

		lib.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"ok":         len(conflicts) == 0,
			"applied":    applied,
			"duplicates": duplicates,
			"stale":      stale,
			"conflicts":  conflicts,
		})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func toTursoArg(v interface{}) lib.TursoArg {
	if v == nil {
		return lib.Null()
	}
	switch t := v.(type) {
	case bool:
		if t {
			return lib.Int(1)
		}
		return lib.Int(0)
	case float64:
		if t == float64(int64(t)) {
			return lib.Int(int64(t))
		}
		return lib.Text(strconv.FormatFloat(t, 'f', -1, 64))
	case int64:
		return lib.Int(t)
	case int:
		return lib.Int(int64(t))
	default:
		return lib.Text(lib.ToString(v))
	}
}
