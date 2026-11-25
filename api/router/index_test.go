package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouterDispatchesKnownRoutes(t *testing.T) {
	cases := []struct {
		target string
		want   int
	}{
		// Without Turso env vars, health answers 503 instead of 404: proof the
		// request reached the health handler. Health is public.
		{"/api/router?__route=health", http.StatusServiceUnavailable},
		// Pairing is public, so it answers the handler's own 405/400 rather
		// than 401.
		{"/api/router?__route=pair", http.StatusMethodNotAllowed},
		// Every data route is protected: with no Authorization header the
		// router answers stealth 404 before any handler or database work.
		{"/api/router?__route=invoices", http.StatusNotFound},
		{"/api/router?__route=products/manage&sku=X", http.StatusNotFound},
		{"/api/router?__route=purchases", http.StatusNotFound},
		{"/api/router?__route=finance", http.StatusNotFound},
		// Unknown routes are also behind auth, so the relay never exposes
		// which route names exist to an anonymous caller.
		{"/api/router?__route=does-not-exist", http.StatusNotFound},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.target, nil)
		rec := httptest.NewRecorder()
		Handler(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s: got %d, want %d (%s)", tc.target, rec.Code, tc.want, rec.Body.String())
		}
	}
}

func TestRouterRejectsBadToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/router?__route=invoices", nil)
	req.Header.Set("Authorization", "Bearer nope")
	rec := httptest.NewRecorder()
	Handler(rec, req)
	// No database configured locally: a well-formed but unknown token cannot
	// be verified, so the router must fail closed (500), never open.
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500", rec.Code)
	}
}

func TestRouterDerivesRouteFromPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("direct /api/health: got %d, want 503", rec.Code)
	}
}
