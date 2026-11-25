package lib

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
)

const DeviceCookieName = "taj_device_token"

var (
	ErrMissingToken = errors.New("authorization required")
	ErrInvalidToken = errors.New("invalid or revoked token")
)

// Real tokens are short (lib.NewToken creates 64 hex characters). Bounding the
// accepted header keeps a hostile caller from making every request hash an
// arbitrarily large blob before the lookup is rejected.
const maxTokenLen = 4096

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
	if r == nil {
		return Identity{}, false
	}
	id, ok := r.Context().Value(identityKey{}).(Identity)
	return id, ok
}

// bearerToken parses and bounds the Authorization header without ever putting
// the raw token in an error message.
func bearerToken(header string) (string, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", ErrMissingToken
	}
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", ErrMissingToken
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return "", ErrMissingToken
	}
	if len(token) > maxTokenLen {
		return "", ErrInvalidToken
	}
	for i := 0; i < len(token); i++ {
		if token[i] < 0x20 || token[i] > 0x7e {
			return "", ErrInvalidToken
		}
	}
	return token, nil
}

// Authenticate accepts either a paired phone device token or a desktop agent
// token. Both are stored as SHA-256 hashes, never in clear text.
func Authenticate(r *http.Request) (Identity, error) {
	if r == nil {
		return Identity{}, ErrMissingToken
	}
	// Header.Get returns only the first value when a client sends duplicate
	// Authorization headers, so one request can never trigger two lookups.
	token, err := bearerToken(r.Header.Get("Authorization"))
	if err != nil {
		// Fall back to HttpOnly paired-device cookie set during /api/pair
		if c, cerr := r.Cookie(DeviceCookieName); cerr == nil && strings.TrimSpace(c.Value) != "" {
			tok := strings.TrimSpace(c.Value)
			if len(tok) <= maxTokenLen {
				token = tok
				err = nil
			}
		}
	}
	if err != nil {
		return Identity{}, err
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

// WriteStealth404 hides protected/unknown mobile relay routes behind a generic
// 404 Not Found so web scanners and browsers cannot tell endpoints exist.
func WriteStealth404(w http.ResponseWriter, r *http.Request) {
	w.Header().Del("WWW-Authenticate")
	http.NotFound(w, r)
}

// RequireAuth authenticates once per request and returns the identity for the
// router to attach via WithIdentity. Unauthenticated / invalid-token callers
// receive a stealth 404 when STEALTH_404 != "0" (default ON).
func RequireAuth(w http.ResponseWriter, r *http.Request) (Identity, bool) {
	id, err := Authenticate(r)
	if err == nil {
		return id, true
	}
	if err == ErrMissingToken || err == ErrInvalidToken {
		if os.Getenv("STEALTH_404") != "0" {
			WriteStealth404(w, r)
			return Identity{}, false
		}
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return Identity{}, false
	}
	http.Error(w, "authentication temporarily unavailable", http.StatusInternalServerError)
	return Identity{}, false
}
