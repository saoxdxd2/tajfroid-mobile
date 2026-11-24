# Production Sync Verification — rollout runbook

This file is a checklist only. Nothing here pushes, deploys, or installs unless
you explicitly approve those steps.

## 1. Local gate

```bash
npm run check:sync
npm run check:offline
npm run check:build
go test ./... -count=1
go test ./api/router -run TestSyncE2E -v -count=1
go build ./...
npm run check:release
```

`check:release` must report `RESULT: READY TO PUSH/DEPLOY WHEN EXPLICITLY APPROVED`.
Set `RELAY_HEALTH_URL=https://mobile.tajfroid.com/api/health` to include a live
probe; it is skipped by default so local work stays offline.

Current known gate: 5 local commits ahead of `origin/main`.

Live pre-deploy probe (2026-11-24, read-only):
`GET https://mobile.tajfroid.com/api/health` → HTTP 500 with
`{"error":"no results: {\"error\":\"JWT error: InvalidToken\"}","ok":false}`.
This is the known token-rotation/deploy blocker; it is not fixed by local code.

## 2. Pre-deploy catalog migration (required once, idempotent)

The relay now reads `catalog.pinned_price`. Fresh databases get the column from
`schema.sql`; existing databases must add it before the relay is deployed.

Guard query (no secret values, read-only):

```sql
SELECT name FROM pragma_table_info('catalog') WHERE name = 'pinned_price';
```

- If the query returns one row, the column exists: do nothing.
- If it returns no rows, run exactly once:

```sql
ALTER TABLE catalog ADD COLUMN pinned_price TEXT NULL;
```

Re-run the guard query afterwards to confirm the row is present. Repeating the
guard is safe; never run the `ALTER` twice (SQLite has no `ADD COLUMN IF NOT
EXISTS`). Supply Turso credentials through the deployment environment, not on
the command line, and never print them. Deploy the relay only after the guard
passes: catalog reads in the new build select `pinned_price` directly.

Semantics after migration: `NULL` = never pinned, `''` = explicitly cleared,
non-empty decimal = authoritative pin. `effective_price` falls back to
`sale_price` for both `NULL` and `''`.

## 3. Relay rollout (requires explicit approval)

```bash
git push origin main
vercel --prod
```

Expected Vercel project: `saos-projects-56035f6d/tajfroid-mobile`.
If Vercel answers `api-deployments-free-per-day`, stop and retry next day; do
not bypass or create a second project.

## 4. Post-deploy verification

```bash
curl -sS https://mobile.tajfroid.com/api/health
```

Expected: HTTP 200, `status: ok`, `checks.database: true`,
`checks.auth_indexes: true`, and both entries under `indexes` true.

If metrics are enabled in the deployment env:

```bash
OBSERVABILITY_METRICS=1  # Vercel env, not a shell secret here
curl -sS "https://mobile.tajfroid.com/api/router?__route=metrics"
```

Check `p50_ms`, `p95_ms`, `max_ms`, `count` per route. No token values may
appear in logs or metrics.

## 5. Phone/relay/desktop sync smoke

1. Pair phone with desktop.
2. Turn off phone network; create a customer and a check.
3. Confirm both appear in the offline queue panel with `pending`.
4. Reconnect; confirm both become `applied`.
5. Repeat the identical create after reconnect; confirm no duplicate row.
6. Edit the same customer from desktop and phone offline; reconnect; confirm a
   conflict is shown and the per-field merge UI loads the server record.
7. Confirm desktop tray shows `last_sync_at` advancing and no plaintext scan
   data in logs.

## 6. APK release (requires manual confirmation)

See `C:\Users\user\tajfroid-apk\RELEASE.md` and run
`scripts/build-release.ps1 -VerifyManifestOnly` first. The current canonical
web manifest drifts from the pinned TWA manifest on `background_color`
(`#ffffff` vs `#1F3A5F`); resolve that before a reproducible signed rebuild.
Signing needs keystore/key passwords supplied through environment variables;
never commit or print them.

Manual install on phone is still required. Confirm camera capture, encrypted
upload, offline queue, and conflict UI inside the installed TWA.
