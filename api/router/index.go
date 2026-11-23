// Package handler is the single Vercel serverless entry point for the whole
// relay API. Vercel's Hobby plan allows at most 12 functions per deployment,
// so every route is dispatched here through one rewrite:
//
//	/api/(.*) -> /api/router?__route=$1
//
// Handlers themselves live in the top-level handlers/ tree and stay small and
// independently testable.
package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	agentbootstrap "tajfroid-mobile/handlers/agent/bootstrap"
	agentcatalog "tajfroid-mobile/handlers/agent/catalog"
	agentheartbeat "tajfroid-mobile/handlers/agent/heartbeat"
	agentpairing "tajfroid-mobile/handlers/agent/pairing"
	agentscanfile "tajfroid-mobile/handlers/agent/scan_file"
	agentscanstatus "tajfroid-mobile/handlers/agent/scan_status"
	agentscans "tajfroid-mobile/handlers/agent/scans"
	brands "tajfroid-mobile/handlers/brands"
	brandsmanage "tajfroid-mobile/handlers/brands-manage"
	catalog "tajfroid-mobile/handlers/catalog"
	categories "tajfroid-mobile/handlers/categories"
	categoriesmanage "tajfroid-mobile/handlers/categories-manage"
	checks "tajfroid-mobile/handlers/checks"
	checksmanage "tajfroid-mobile/handlers/checks-manage"
	customers "tajfroid-mobile/handlers/customers"
	customersmanage "tajfroid-mobile/handlers/customers-manage"
	devices "tajfroid-mobile/handlers/devices"
	devicesmanage "tajfroid-mobile/handlers/devices-manage"
	expenses "tajfroid-mobile/handlers/expenses"
	expensesmanage "tajfroid-mobile/handlers/expenses-manage"
	finance "tajfroid-mobile/handlers/finance"
	health "tajfroid-mobile/handlers/health"
	invoices "tajfroid-mobile/handlers/invoices"
	invoicesitems "tajfroid-mobile/handlers/invoices-items"
	invoicesmanage "tajfroid-mobile/handlers/invoices-manage"
	journal "tajfroid-mobile/handlers/journal"
	pair "tajfroid-mobile/handlers/pair"
	payments "tajfroid-mobile/handlers/payments"
	products "tajfroid-mobile/handlers/products"
	productsmanage "tajfroid-mobile/handlers/products-manage"
	purchases "tajfroid-mobile/handlers/purchases"
	purchasesitems "tajfroid-mobile/handlers/purchases-items"
	purchasesmanage "tajfroid-mobile/handlers/purchases-manage"
	scans "tajfroid-mobile/handlers/scans"
	stock "tajfroid-mobile/handlers/stock"
	suppliers "tajfroid-mobile/handlers/suppliers"
	suppliersmanage "tajfroid-mobile/handlers/suppliers-manage"
	"tajfroid-mobile/lib"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	route := r.URL.Query().Get("__route")
	if route == "" {
		// Local development / direct invocations: derive the route from the
		// path as well so the router stays usable outside Vercel.
		route = strings.TrimPrefix(r.URL.Path, "/api/")
		route = strings.TrimPrefix(route, "router")
		route = strings.TrimPrefix(route, "/")
	}
	route = strings.Trim(route, "/")

	// Bound the whole fan-out so a hung Turso call cannot hold a Vercel
	// invocation until the 15s HTTP backstop.
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	r = r.WithContext(ctx)

	// Public endpoints: health is the uptime probe; pair consumes a one-time
	// code; agent/bootstrap only works while no agent is registered. Every
	// other route requires a paired device or agent token.
	switch route {
	case "health", "pair", "agent/bootstrap":
	default:
		id, ok := lib.RequireAuth(w, r)
		if !ok {
			return
		}
		// Handlers reuse the verified identity instead of re-querying tokens.
		r = lib.WithIdentity(r, id)
	}

	// Dynamic agent scan routes: /api/agent/scans/{id}/status|file
	if strings.HasPrefix(route, "agent/scans/") {
		rest := strings.TrimPrefix(route, "agent/scans/")
		if id, ok := strings.CutSuffix(rest, "/status"); ok {
			withID(r, id)
			agentscanstatus.Handler(w, r)
			return
		}
		if id, ok := strings.CutSuffix(rest, "/file"); ok {
			withID(r, id)
			agentscanfile.Handler(w, r)
			return
		}
	}

	switch route {
	case "health":
		health.Handler(w, r)
	case "pair":
		pair.Handler(w, r)
	case "catalog":
		catalog.Handler(w, r)
	case "scans":
		scans.Handler(w, r)

	case "agent/bootstrap":
		agentbootstrap.Handler(w, r)
	case "agent/catalog":
		agentcatalog.Handler(w, r)
	case "agent/heartbeat":
		agentheartbeat.Handler(w, r)
	case "agent/pairing":
		agentpairing.Handler(w, r)
	case "agent/scans":
		agentscans.Handler(w, r)
	case "agent/scan_status":
		agentscanstatus.Handler(w, r)
	case "agent/scan_file":
		agentscanfile.Handler(w, r)

	case "products":
		products.Handler(w, r)
	case "products/manage":
		productsmanage.Handler(w, r)
	case "stock":
		stock.Handler(w, r)
	case "categories":
		categories.Handler(w, r)
	case "categories/manage":
		categoriesmanage.Handler(w, r)
	case "brands":
		brands.Handler(w, r)
	case "brands/manage":
		brandsmanage.Handler(w, r)

	case "customers":
		customers.Handler(w, r)
	case "customers/manage":
		customersmanage.Handler(w, r)
	case "suppliers":
		suppliers.Handler(w, r)
	case "suppliers/manage":
		suppliersmanage.Handler(w, r)

	case "invoices":
		invoices.Handler(w, r)
	case "invoices/manage":
		invoicesmanage.Handler(w, r)
	case "invoices/items":
		invoicesitems.Handler(w, r)
	case "payments":
		payments.Handler(w, r)

	case "purchases":
		purchases.Handler(w, r)
	case "purchases/manage":
		purchasesmanage.Handler(w, r)
	case "purchases/items":
		purchasesitems.Handler(w, r)

	case "checks":
		checks.Handler(w, r)
	case "checks/manage":
		checksmanage.Handler(w, r)
	case "expenses":
		expenses.Handler(w, r)
	case "expenses/manage":
		expensesmanage.Handler(w, r)
	case "finance":
		finance.Handler(w, r)
	case "journal":
		journal.Handler(w, r)
	case "devices":
		devices.Handler(w, r)
	case "devices/manage":
		devicesmanage.Handler(w, r)

	default:
		http.NotFound(w, r)
	}
}

// withID injects the captured scan id as a query parameter, matching the
// shape the scan_file/scan_status handlers already expect.
func withID(r *http.Request, id string) {
	q := r.URL.Query()
	q.Set("id", id)
	r.URL.RawQuery = q.Encode()
}
