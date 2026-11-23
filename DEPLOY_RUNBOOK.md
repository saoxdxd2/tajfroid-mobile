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

Current known gate: 4 local commits ahead of `origin/main`.

## 2. Relay rollout (requires explicit approval)

```bash
git push origin main
vercel --prod
```

Expected Vercel project: `saos-projects-56035f6d/tajfroid-mobile`.
If Vercel answers `api-deployments-free-per-day`, stop and retry next day; do
not bypass or create a second project.

## 3. Post-deploy verification

```bash
curl -sS https://mobile.tajfroid.com/api/health
```

Expected: HTTP 200, `status: ok`, `turso: ok`, both auth indexes present.

If metrics are enabled in the deployment env:

```bash
OBSERVABILITY_METRICS=1  # Vercel env, not a shell secret here
curl -sS "https://mobile.tajfroid.com/api/router?__route=metrics"
```

Check `p50_ms`, `p95_ms`, `max_ms`, `count` per route. No token values may
appear in logs or metrics.

## 4. Phone/relay/desktop sync smoke

1. Pair phone with desktop.
2. Turn off phone network; create a customer and a check.
3. Confirm both appear in the offline queue panel with `pending`.
4. Reconnect; confirm both become `applied`.
5. Repeat the identical create after reconnect; confirm no duplicate row.
6. Edit the same customer from desktop and phone offline; reconnect; confirm a
   conflict is shown and the per-field merge UI loads the server record.
7. Confirm desktop tray shows `last_sync_at` advancing and no plaintext scan
   data in logs.

## 5. APK release (requires manual confirmation)

See `C:\Users\user\tajfroid-apk\RELEASE.md` once the agent-prepared release
script is present. Signing needs keystore/key passwords supplied through
environment variables; never commit or print them.

Manual install on phone is still required. Confirm camera capture, encrypted
upload, offline queue, and conflict UI inside the installed TWA.
