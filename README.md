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
1. Create a Turso database and run `relay/schema.sql`.
2. Set env vars: `TURSO_DATABASE_URL`, `TURSO_AUTH_TOKEN`, `ERP_SETUP_SECRET`.
3. Deploy `relay/` to Vercel (separate project from tajfroid.com). Vercel builds the Go functions automatically.
4. Point `mobile.tajfroid.com` at the Vercel project.

Note: the relay is written in Go. Local `go build ./...` cannot traverse the
`[id]` dynamic-route directories (Vercel convention); use `go vet` on
individual files or build via Vercel. The shared logic lives in `relay/lib/`.

## Security notes
- Relay stores only ciphertext and status events; never plaintext photos or financial data.
- Device tokens are stored hashed (sha256) on the relay; desktop stores only sha256(token).
- Desktop X25519 private key lives in Windows Credential Manager (user-initiated encrypted backup documented).
- E2E encryption: a lost desktop key makes unprocessed scans unrecoverable. Back up the key.