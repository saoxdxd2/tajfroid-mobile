import { createServer } from 'vite'
import { createPinia, setActivePinia } from 'pinia'

// Runtime proof for the phone-side mutation queue. It uses a tiny in-memory
// IndexedDB and a stubbed relay, so no browser, network, or secret is needed.
class MemoryIndexedDB {
  constructor() {
    this.stores = new Map([['mutations', new Map()]])
    this.db = {
      transaction: (name, mode) => this.transaction(name, mode),
      objectStoreNames: { contains: name => this.stores.has(name) },
      createObjectStore: name => {
        this.stores.set(name, new Map())
        return { createIndex: () => {} }
      },
    }
  }

  open() {
    const req = {}
    queueMicrotask(() => {
      req.result = this.db
      req.onsuccess?.()
    })
    return req
  }

  transaction(name) {
    const map = this.stores.get(name)
    const tx = {}
    tx.objectStore = () => ({
      getAll: () => {
        const req = {}
        queueMicrotask(() => {
          req.result = [...map.values()].map(x => structuredClone(x))
          req.onsuccess?.()
        })
        return req
      },
      put: item => {
        map.set(item.id, structuredClone(item))
        queueMicrotask(() => tx.oncomplete?.())
      },
      delete: id => {
        map.delete(id)
        queueMicrotask(() => tx.oncomplete?.())
      },
    })
    return tx
  }
}

const failures = []
const checks = []
const check = (name, ok, detail = '') => (ok ? checks : failures).push(`${name}${detail ? ` — ${detail}` : ''}`)
const tick = () => new Promise(resolve => setTimeout(resolve, 0))
const response = (status, body) => ({
  status,
  ok: status >= 200 && status < 300,
  json: async () => body,
  text: async () => JSON.stringify(body),
})

globalThis.indexedDB ||= new MemoryIndexedDB()
globalThis.navigator ||= { onLine: true }
globalThis.crypto ||= {
  randomUUID: () => `${Date.now().toString(16)}-${Math.random().toString(36).slice(2)}`,
}
const apiCalls = []
globalThis.__offlineApi = { impl: async () => response(200, {}) }

const vite = await createServer({
  configFile: false,
  root: process.cwd(),
  logLevel: 'silent',
  appType: 'custom',
  server: { middlewareMode: true },
  plugins: [{
    name: 'stub-relay-api-for-offline-test',
    enforce: 'pre',
    resolveId(source) {
      if (source.includes('lib/api')) return '\0test-relay-api'
    },
    load(id) {
      if (id !== '\0test-relay-api') return
      return `
        export async function apiFetch(path, options) {
          globalThis.__apiCalls.push({ path, options })
          return globalThis.__offlineApi.impl(path, options)
        }
        export async function readError(res, fallback) { return fallback }
      `
    },
  }],
})

try {
  globalThis.__apiCalls = apiCalls
  const { useMutationStore } = await vite.ssrLoadModule('/src/stores/mutations.js')
  setActivePinia(createPinia())
  const store = useMutationStore()
  await store.load()

  // 1. Offline create stays in IndexedDB and sends nothing.
  navigator.onLine = false
  const createId = await store.enqueue({
    method: 'POST', path: '/api/customers',
    body: { id: 'c-100', name: 'Offline Customer' },
    entity: 'customers', entityId: 'c-100', idempotencyKey: 'customer:c-100',
  })
  await tick()
  check('offline create is queued', store.items.some(x => x.id === createId && x.status === 'pending'))
  check('offline create sends no request', apiCalls.length === 0, `calls=${apiCalls.length}`)

  // 2. Reconnect replays once and records the server id.
  navigator.onLine = true
  globalThis.__offlineApi.impl = async () => response(201, { id: 'c-100' })
  await store.flush()
  check('reconnect replays create', apiCalls.length === 1, `calls=${apiCalls.length}`)
  const applied = store.items.find(x => x.id === createId)
  check('create marked applied', applied?.status === 'applied')
  check('server id stored for replay guard', applied?.serverId === 'c-100')

  // 3. Identical create replay is skipped locally (server is also idempotent).
  await store.enqueue({
    method: 'POST', path: '/api/customers',
    body: { id: 'c-100', name: 'Offline Customer' },
    entity: 'customers', entityId: 'c-100', idempotencyKey: 'customer:c-100',
  })
  await tick()
  await store.flush()
  check('duplicate enqueue reuses queue item', store.items.filter(x => x.idempotencyKey === 'customer:c-100').length === 1)
  check('duplicate create sends no second request', apiCalls.length === 1, `calls=${apiCalls.length}`)

  // 4. Stale update becomes a conflict with the server token preserved.
  globalThis.__offlineApi.impl = async () => response(409, {
    error: 'Conflict',
    currentUpdatedAt: 'server-v2',
  })
  const staleId = await store.enqueue({
    method: 'PUT', path: '/api/customers/manage',
    body: { id: 'c-100', name: 'Phone Edit' },
    entity: 'customers', entityId: 'c-100',
    baseUpdatedAt: 'server-v1',
  })
  await tick()
  const conflicted = store.items.find(x => x.id === staleId)
  check('stale update becomes conflict', conflicted?.status === 'conflict', `status=${conflicted?.status}`)
  check('conflict keeps local edit', conflicted?.body?.name === 'Phone Edit')
  check('conflict keeps merge token', conflicted?.conflict?.currentUpdatedAt === 'server-v2')

  // 5. Re-enqueue with the fetched base succeeds and does not discard payload.
  await store.remove(staleId)
  globalThis.__offlineApi.impl = async () => response(200, { id: 'c-100' })
  const retryId = await store.enqueue({
    method: 'PUT', path: '/api/customers/manage',
    body: { id: 'c-100', name: 'Phone Edit' },
    entity: 'customers', entityId: 'c-100',
    baseUpdatedAt: 'server-v2',
  })
  await tick()
  const retried = store.items.find(x => x.id === retryId)
  check('retry with current base succeeds', retried?.status === 'applied', `status=${retried?.status}`)
  const retryCall = apiCalls.at(-1)
  check('retry sends current base token', JSON.parse(retryCall.options.body).baseUpdatedAt === 'server-v2')
  check('retry preserves edited field', JSON.parse(retryCall.options.body).name === 'Phone Edit')

  await store.load()
  check('queue survives reload', store.items.some(x => x.id === retryId && x.status === 'applied'))
} finally {
  await vite.close()
}

console.log(`PASS ${checks.length}`)
for (const c of checks) console.log(`  ok   ${c}`)
if (failures.length) {
  console.log(`\nFAIL ${failures.length}`)
  for (const f of failures) console.log(`  FAIL ${f}`)
  process.exit(1)
}
