package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSyncHLCPushAppliesDeltaAndDetectsFieldConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Requests []struct {
				Stmt struct {
					SQL string `json:"sql"`
				} `json:"stmt"`
			} `json:"requests"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		results := make([]map[string]interface{}, 0, len(body.Requests))
		for _, req := range body.Requests {
			sql := req.Stmt.SQL
			rows := [][]interface{}{}
			if strings.Contains(sql, "SELECT hlc, device_id, value_json FROM sync_field_clocks") {
				rows = [][]interface{}{
					{"0001700000005000:000001:pc-1", "pc-1", `"1200"`},
				}
			}
			results = append(results, map[string]interface{}{
				"type": "execute",
				"response": map[string]interface{}{
					"result": map[string]interface{}{
						"columns":            []string{},
						"rows":               rows,
						"affected_row_count": 1,
					},
				},
			})
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"results": results})
	}))
	defer srv.Close()
	t.Setenv("TURSO_DATABASE_URL", srv.URL)
	t.Setenv("TURSO_AUTH_TOKEN", "sync-test")

	payload := map[string]interface{}{
		"ops": []map[string]interface{}{
			{
				"opId":     "op-delta-1",
				"deviceId": "phone-1",
				"entity":   "catalog",
				"entityId": "75P7L",
				"opKind":   "stock_delta",
				"deltaInt": -1,
				"hlc":      "0001700000006000:000000:phone-1",
			},
			{
				"opId":     "op-conflict-1",
				"deviceId": "phone-1",
				"entity":   "catalog",
				"entityId": "75P7L",
				"opKind":   "field_set",
				"field":    "pinned_price",
				"value":    "1300",
				"hlc":      "0001700000006000:000001:phone-1",
				"baseHlc":  "0001700000001000:000000:phone-1",
			},
		},
	}
	raw, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/sync/hlc", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	Handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		OK        bool                     `json:"ok"`
		Applied   []string                 `json:"applied"`
		Conflicts []map[string]interface{} `json:"conflicts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.OK || len(out.Applied) != 1 || out.Applied[0] != "op-delta-1" || len(out.Conflicts) != 1 {
		t.Fatalf("unexpected sync response: %+v", out)
	}
}
