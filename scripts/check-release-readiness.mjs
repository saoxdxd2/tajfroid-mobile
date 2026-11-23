import fs from 'node:fs'
import path from 'node:path'
import { spawnSync } from 'node:child_process'

// Local go/no-go checklist for deploying the relay. It never prints secrets
// and never pushes or deploys anything.
const root = process.cwd()
const hard = []
const soft = []
const ok = (name, detail = '') => console.log(`  ok   ${name}${detail ? ` — ${detail}` : ''}`)
const fail = (name, detail = '') => { console.log(`  FAIL ${name}${detail ? ` — ${detail}` : ''}`); hard.push(name) }
const note = (name, detail = '') => { console.log(`  note ${name}${detail ? ` — ${detail}` : ''}`); soft.push(name) }

function run(cmd, args) {
  return spawnSync(cmd, args, { cwd: root, encoding: 'utf8' })
}

console.log('Release readiness (local checks only)\n')

// 1. Working tree and unpushed work.
const status = run('git', ['status', '--porcelain'])
if (status.status !== 0) fail('git status', status.stderr?.trim())
else if (status.stdout.trim()) fail('working tree clean', `${status.stdout.trim().split('\n').length} changed file(s)`)
else ok('working tree clean')

const ahead = run('git', ['rev-list', '--count', 'origin/main..HEAD'])
if (ahead.status === 0) {
  const n = Number(ahead.stdout.trim())
  if (n > 0) note('unpushed commits', `${n} commit(s) ready for explicit push approval`)
  else ok('no unpushed commits')
} else note('remote tracking', 'origin/main not available locally')

// 2. Built artifacts from the last verified build.
for (const artifact of ['dist/index.html', 'dist/assets/libsodium-wrappers-D8hiYJjQ.js']) {
  if (fs.existsSync(path.join(root, artifact))) ok('artifact', artifact)
  else fail('artifact missing', artifact)
}

// 3. Secrets/tooling presence only — never read or print values.
const tokenFile = 'C:\\Users\\user\\Documents\\TAJFROID\\taj_dbtoken_0.txt'
if (fs.existsSync(tokenFile)) ok('Turso token file present', 'path only; value not read')
else note('Turso token file missing', 'set TURSO_AUTH_TOKEN in the deploy environment')

const cli = run(process.platform === 'win32' ? 'where' : 'which', ['vercel'])
if (cli.status === 0) ok('Vercel CLI available')
else note('Vercel CLI not on PATH', 'WSL login may be required before deploy')

// 4. APK/TWA release inputs.
const apkRoot = 'C:\\Users\\user\\tajfroid-apk'
for (const artifact of ['twa-manifest.json', 'android.keystore', 'app/build/outputs/apk/release/app-release-unsigned.apk']) {
  if (fs.existsSync(path.join(apkRoot, artifact))) ok('APK input', artifact)
  else note('APK input missing', artifact)
}

// 5. Optional live health probe, only when explicitly requested.
const healthUrl = process.env.RELAY_HEALTH_URL || ''
if (!healthUrl) note('live health probe skipped', 'set RELAY_HEALTH_URL to enable')
else {
  const res = run('curl', ['-sS', '-o', 'NUL', '-w', '%{http_code}', '--max-time', '10', healthUrl])
  const code = res.stdout.trim()
  if (res.status === 0 && code === '200') ok('live health probe', `HTTP ${code}`)
  else fail('live health probe', `HTTP ${code || 'no response'}`)
}

console.log(`\nRESULT: ${hard.length ? `BLOCKED (${hard.length} hard failure(s))` : 'READY TO PUSH/DEPLOY WHEN EXPLICITLY APPROVED'}`)
if (soft.length) console.log(`Notes: ${soft.length}`)
process.exit(hard.length ? 1 : 0)
