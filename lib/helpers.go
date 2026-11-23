package lib

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

func NewID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func NewToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func Sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func Unwrap(v interface{}) interface{} {
	m, ok := v.(map[string]interface{})
	if !ok {
		return v
	}
	t, _ := m["type"].(string)
	val, hasVal := m["value"]
	if !hasVal {
		if t == "null" {
			return nil
		}
		return v
	}
	switch t {
	case "null":
		return nil
	case "text", "integer", "real", "blob":
		return val
	default:
		return v
	}
}

func ToString(v interface{}) string {
	v = Unwrap(v)
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	default:
		return fmt.Sprintf("%v", x)
	}
}

func ToInt(v interface{}) int {
	v = Unwrap(v)
	switch x := v.(type) {
	case float64:
		return int(x)
	case int64:
		return int(x)
	case int:
		return x
	case string:
		n, _ := strconv.Atoi(x)
		return n
	default:
		return 0
	}
}

func ToInt64(v interface{}) int64 {
	v = Unwrap(v)
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	case string:
		n, _ := strconv.ParseInt(x, 10, 64)
		return n
	default:
		return 0
	}
}

func NullStr(v interface{}) string {
	if v == nil {
		return ""
	}
	s := ToString(v)
	if s == "<nil>" {
		return ""
	}
	return s
}

func NullIface(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	return ToString(v)
}

func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func WriteConflict(w http.ResponseWriter, currentUpdatedAt int64) {
	WriteJSON(w, http.StatusConflict, map[string]interface{}{
		"error":            "conflict: record changed on another device",
		"currentUpdatedAt": currentUpdatedAt,
	})
}