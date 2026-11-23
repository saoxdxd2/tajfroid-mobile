package lib

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

var (
	ErrMissingToken = errors.New("authorization required")
	ErrInvalidToken = errors.New("invalid or revoked token")
)

type Identity struct {
	Kind string
	ID   string
}

type identityKey struct{}

// WithIdentity stores the verified identity on the request context so handlers
// never repeat the auth database lookup. Only the router may set this value.
func WithIdentity(r *http.Request, id Identity) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), identityKey{}, id))
}

func IdentityFrom(r *http.Request) (Identity, bool) {
	id, ok := r.Context().Value(identityKey{}).(Identity)
	return id, ok
}

// Authenticate accepts either a paired phone device token or a desktop agent
// token. Both are stored as SHA-256 hashes, never in clear text.
func Authenticate(r *http.Request) (Identity, error) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return Identity{}, ErrMissingToken
	}
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return Identity{}, ErrMissingToken
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return Identity{}, ErrMissingToken
	}
	hash := Sha256Hex(token)
	db := NewTurso()
	res, err := db.ExecuteContext(r.Context(),
		`SELECT 'agent', id FROM agent_tokens WHERE token_hash = ?
		 UNION ALL
		 SELECT 'device', id FROM devices WHERE device_token_hash = ? AND revoked_at IS NULL
		 LIMIT 1`,
		Text(hash), Text(hash))
	if err != nil {
		return Identity{}, err
	}
	if len(res.Rows) == 0 {
		return Identity{}, ErrInvalidToken
	}
	return Identity{Kind: ToString(res.Rows[0][0]), ID: ToString(res.Rows[0][1])}, nil
}

// RequireAuth authenticates once per request and returns the identity for the
// router to attach via WithIdentity.
func RequireAuth(w http.ResponseWriter, r *http.Request) (Identity, bool) {
	id, err := Authenticate(r)
	if err == nil {
		return id, true
	}
	status := http.StatusUnauthorized
	if err != ErrMissingToken && err != ErrInvalidToken {
		status = http.StatusInternalServerError
	}
	http.Error(w, err.Error(), status)
	return Identity{}, false
}
