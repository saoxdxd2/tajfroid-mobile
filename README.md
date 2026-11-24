# Taj-Froid Mobile (PWA)

Installable mobile PWA at `mobile.tajfroid.com`. Captures supplier papers
with the phone camera and syncs through a small relay. The desktop ERP
remains the only place that OCRs, reviews and posts.

## Repos
- This repo: phone PWA + relay API (Vercel + Turso + private Blob).
- Desktop agent: `taj_app/src/modules/mobile_sync/` (this repo is consumed by it).
- `tajfroid` (website) is untouched.

## Structure
- `src/` — Vue 3 + Pinia PWA (Vite).
- `relay/` — Vercel functions (Node) + Turso schema (`relay/schema.sql`).

## Setup (PWA)
```bash
npm install
npm run dev
```

## Performance checks

The production build is measured directly through Vite (npm adds 0.5–0.8s of
process startup before Vite loads):

```bash
node node_modules/vite/bin/vite.js build   # typical: 0.35–0.45s Vite time
npm run check:build                        # 5 direct builds, fails if any > 2s
npm run check:sync                         # offline/conflict/perf invariants
npm run check:offline                      # runtime queue replay/conflict proof
go test ./...                              # relay + latency/round-trip guards
go test ./lib -run TestWarmExecuteLatencyAndConnectionReuse -v
```

`libsodium-wrappers` is intentionally a lazy chunk: it is loaded only when a
scan is encrypted, never on app start.

## Offline app shell

In production the `/sw.js` service worker caches only the app shell (HTML,
manifest, icons and hashed JS/CSS assets, including the lazy libsodium chunk)
after the first online load, so the UI works offline afterwards. API data —
including auth, QR lookups and ciphertext — is never cached.

To remove the offline shell, unregister the service worker in DevTools
(Application → Service Workers → Unregister) or call `unregisterAppShell()`
from `src/lib/pwa.js`; that also deletes the `tajfroid-shell-*` caches.

## Setup (relay, Go)

### Get your Turso URL and token
1. Install the Turso CLI: https://docs.turso.tech/cli/installation
2. Sign up / log in: `turso auth login`
3. Create a database: `turso db create tajfroid-mobile`
4. Get the URL: `turso db show tajfroid-mobile --url`  → this is `TURSO_DATABASE_URL`
5. Get the token: `turso db tokens create tajfroid-mobile`  → this is `TURSO_AUTH_TOKEN`
6. Run the schema: `turso db shell tajfroid-mobile < schema.sql`

### Deploy
1. Create a Turso database and run `schema.sql`.
2. Set env vars in Vercel:
   - `TURSO_DATABASE_URL` — your Turso database URL (see below)
   - `TURSO_AUTH_TOKEN` — your Turso auth token
   - `OBSERVABILITY_METRICS` — optional; set to `1` to expose the metrics route
   - (ERP_SETUP_SECRET is no longer needed — auto-connect uses device fingerprint)
3. Deploy to Vercel. Vercel builds the Go functions automatically.
4. Point `mobile.tajfroid.com` at the Vercel project.

### Observability

- Every relay request receives an `X-Request-ID` response header. A printable
  inbound id of at most 128 characters is reused; anything else is replaced by
  a fresh id. The id is also available on the request context.
- Each completed request emits one JSON line to stdout with `timestamp`,
  `requestId`, `route`, `method`, `status`, `duration_ms`, and `auth_kind` /
  `error_category` when known. Tokens, bodies and raw error text are never
  logged.
- `GET /api/router?__route=metrics` returns in-memory `p50_ms`, `p95_ms`,
  `max_ms` and `count` for the last 1000 samples per route+status — only when
  `OBSERVABILITY_METRICS=1`. Without the flag the route answers 404.
- `/api/health` verifies `SELECT 1` and the presence of `idx_devices_token_hash`
  and `idx_agent_tokens_token_hash`, returning `status` (`ok`/`degraded`),
  `checks`, `indexes` and `latency_ms`. It answers 200 when healthy and 503
  otherwise; it never prints the Turso token.

Note: the relay is written in Go. Local `go build ./...` cannot traverse the
`[id]` dynamic-route directories (Vercel convention); use `go vet` on
individual files or build via Vercel. The shared logic lives in `relay/lib/`.

## Security notes
- Relay stores only ciphertext and status events; never plaintext photos or financial data.
- Device tokens are stored hashed (sha256) on the relay; desktop stores only sha256(token).
- Desktop X25519 private key lives in Windows Credential Manager (user-initiated encrypted backup documented).
- E2E encryption: a lost desktop key makes unprocessed scans unrecoverable. Back up the key.