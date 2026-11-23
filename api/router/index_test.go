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
		// request reached the health handler.
		{"/api/router?__route=health", http.StatusServiceUnavailable},
		// Any database-backed route answers 500 (not 404) with no DB configured.
		{"/api/router?__route=invoices", http.StatusInternalServerError},
		// Handlers that translate "no row" into 404 prove dispatch too: the
		// unknown-route case below has the standard "404 page not found" body.
		{"/api/router?__route=products/manage&sku=X", http.StatusNotFound},
		{"/api/router?__route=purchases", http.StatusInternalServerError},
		// Unknown routes must stay 404.
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

func TestRouterDerivesRouteFromPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("direct /api/health: got %d, want 503", rec.Code)
	}
}
