import fs from 'node:fs'
import path from 'node:path'

const root = process.cwd()
const read = p => fs.readFileSync(path.join(root, p), 'utf8')
const failures = []
const checks = []
const check = (name, ok, detail = '') => (ok ? checks : failures).push(`${name}${detail ? ` — ${detail}` : ''}`)

// 1. Server-side optimistic concurrency on every manage/stateful handler.
const stateful = [
  ...fs.readdirSync(path.join(root, 'handlers')).filter(d => d.endsWith('-manage')),
  'invoices-items', 'purchases-items', 'stock',
]
for (const dir of stateful) {
  const s = read(`handlers/${dir}/index.go`)
  check(`${dir}: base token`, /baseUpdatedAt|baseCatalogVersion|baseStock|baseRevokedAt/i.test(s))
  check(`${dir}: 409 response`, /StatusConflict|WriteConflict|writeConflict/.test(s))
}

// 2. Create endpoints must be replay-safe (client id or natural sku key).
const creates = ['brands', 'categories', 'checks', 'customers', 'expenses', 'invoices', 'payments', 'products', 'purchases', 'suppliers']
for (const dir of creates) {
  const s = read(`handlers/${dir}/index.go`)
  check(`${dir}: idempotent create`, /body\.ID\s*!=\s*""|WHERE sku=\?/.test(s))
}

// 3. Every phone mutation action must be queued offline; every update/delete
// must also resolve 409s. Pairing and scan upload are separate auth/blob flows.
const entityStores = ['customers', 'finance', 'inventory', 'purchases', 'sales', 'suppliers']
for (const name of entityStores) {
  const s = read(`src/stores/${name}.js`)
  const actions = [...s.matchAll(/async function\s+(\w+)\s*\([^)]*\)\s*\{([\s\S]*?)(?=\n  (?:async )?function |\n  return \{)/g)]
  for (const [, action, body] of actions) {
    const methods = [...new Set([...body.matchAll(/method:\s*'(POST|PUT|DELETE)'/g)].map(m => m[1]))]
    if (!methods.length) continue
    check(`${name}.${action}: offline queue`, /enqueue\(/.test(body))
    if (methods.some(m => m === 'PUT' || m === 'DELETE')) {
      check(`${name}.${action}: conflict handling`, /conflictFromResponse|recordConflict/.test(body))
    }
  }
}

// 4. Queue core invariants.
const mutations = read('src/stores/mutations.js')
for (const token of ['seq:', 'idempotencyKey', 'serverId', 'applied', 'pruneApplied', 'conflictFromResponse', 'statusCounts']) {
  check(`mutations.js: ${token}`, mutations.includes(token))
}

// 5. Device revoke is a first-class conflicted mutation with a mergeable read.
const account = read('src/views/AccountView.vue')
check('AccountView: revoke base token', account.includes('baseRevokedAt'))
check('AccountView: revoke conflict handling', account.includes('conflictFromResponse'))
const status = read('src/components/MutationStatus.vue')
check('MutationStatus: device read path', status.includes('/api/devices/manage?id='))
check('MutationStatus: hides applied rows', status.includes("i.status !== 'applied'"))

console.log(`PASS ${checks.length}`)
for (const c of checks) console.log(`  ok   ${c}`)
if (failures.length) {
  console.log(`\nFAIL ${failures.length}`)
  for (const f of failures) console.log(`  FAIL ${f}`)
  process.exit(1)
}
