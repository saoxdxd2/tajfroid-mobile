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
   - (ERP_SETUP_SECRET is no longer needed — auto-connect uses device fingerprint)
3. Deploy to Vercel. Vercel builds the Go functions automatically.
4. Point `mobile.tajfroid.com` at the Vercel project.

Note: the relay is written in Go. Local `go build ./...` cannot traverse the
`[id]` dynamic-route directories (Vercel convention); use `go vet` on
individual files or build via Vercel. The shared logic lives in `relay/lib/`.

## Security notes
- Relay stores only ciphertext and status events; never plaintext photos or financial data.
- Device tokens are stored hashed (sha256) on the relay; desktop stores only sha256(token).
- Desktop X25519 private key lives in Windows Credential Manager (user-initiated encrypted backup documented).
- E2E encryption: a lost desktop key makes unprocessed scans unrecoverable. Back up the key.