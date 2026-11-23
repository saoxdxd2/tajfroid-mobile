import { createServer } from 'vite'
import { createPinia, setActivePinia } from 'pinia'

// Runtime chaos proof for the phone client. Everything is in memory: a fake
// IndexedDB (including corrupt rows, index reads and injected write failures),
// a stubbed relay and a seeded PRNG. No browser, network or secret is needed.
class FakeIndexedDB {
  constructor() {
    this.databases = new Map()
    this.stores = new Map() // `${db}:${store}` -> { keyPath, indexes: Map }
    this.data = new Map() // `${db}:${store}` -> Map(key, value)
    this.failWrites = false
    this.writeFailures = 0
  }

  storeKey(db, name) { return `${db}:${name}` }

  open(name, version) {
    const req = {}
    queueMicrotask(() => {
      let db = this.databases.get(name)
      if (!db) {
        db = {
          version,
          objectStoreNames: { contains: n => this.stores.has(this.storeKey(name, n)) },
          createObjectStore: (n, options = {}) => {
            const meta = { keyPath: options.keyPath || 'id', indexes: new Map() }
            meta.createIndex = (indexName, keyPath) => meta.indexes.set(indexName, keyPath)
            this.stores.set(this.storeKey(name, n), meta)
            this.data.set(this.storeKey(name, n), new Map())
            return meta
          },
          transaction: (stores, mode) => this.transaction(name, stores, mode),
          close: () => {},
        }
        this.databases.set(name, db)
        req.oldVersion = 0
        req.result = db
        req.transaction = { objectStore: n => this.stores.get(this.storeKey(name, n)) }
        queueMicrotask(() => req.onupgradeneeded?.())
      } else {
        req.result = db
      }
      queueMicrotask(() => req.onsuccess?.())
    })
    return req
  }

  transaction(dbName, storeNames, mode) {
    const names = Array.isArray(storeNames) ? storeNames : [storeNames]
    const tx = { error: null, oncomplete: null, onerror: null, onabort: null }
    let pending = 0
    let done = false
    const settle = () => {
      if (--pending === 0 && !done) {
        done = true
        queueMicrotask(() => {
          if (tx.error) { tx.onerror?.(tx.error); tx.onabort?.(tx.error) }
          else tx.oncomplete?.()
        })
      }
    }
    const failWrite = () => {
      if (!this.failWrites || mode !== 'readwrite') return false
      this.writeFailures++
      tx.error = new Error('simulated IndexedDB write failure')
      return true
    }
    tx.objectStore = name => {
      const key = this.storeKey(dbName, name)
      const meta = this.stores.get(key) || { keyPath: 'id', indexes: new Map() }
      const map = this.data.get(key) || new Map()
      this.data.set(key, map)
      const keyOf = value => (value && typeof value === 'object' ? value[meta.keyPath] : undefined)
      const request = run => {
        const req = { error: null, onsuccess: null, onerror: null }
        pending++
        queueMicrotask(() => {
          try {
            const result = run()
            if (tx.error) { req.error = tx.error; req.onerror?.(tx.error) }
            else { req.result = result; req.onsuccess?.() }
          } catch (e) {
            tx.error = e
            req.error = e
            req.onerror?.(e)
          } finally {
            settle()
          }
        })
        return req
      }
      return {
        put: value => request(() => {
          if (failWrite()) return undefined
          const k = keyOf(value)
          if (k !== undefined && k !== null) map.set(k, structuredClone(value))
          return k
        }),
        delete: k => request(() => {
          if (failWrite()) return undefined
          map.delete(k)
          return undefined
        }),
        get: k => request(() => (map.has(k) ? structuredClone(map.get(k)) : undefined)),
        getAll: () => request(() => [...map.values()].map(v => (v == null ? v : structuredClone(v)))),
        index: indexName => ({
          getAll: range => request(() => {
            const keyPath = meta.indexes.get(indexName)
            const wanted = range?.only
            const rows = [...map.values()]
            if (keyPath === undefined || wanted === undefined) return rows.map(v => (v == null ? v : structuredClone(v)))
            return rows
              .filter(v => v && typeof v === 'object' && v[keyPath] === wanted)
              .map(v => structuredClone(v))
          }),
        }),
      }
    }
    return tx
  }

  // Create store metadata directly (no upgrade needed) and seed rows.
  defineStore(dbName, storeName, keyPath, indexPaths = {}) {
    const key = this.storeKey(dbName, storeName)
    const meta = { keyPath, indexes: new Map(Object.entries(indexPaths || {})) }
    meta.createIndex = (name, path) => meta.indexes.set(name, path)
    this.stores.set(key, meta)
    this.data.set(key, new Map())
    return meta
  }

  seed(dbName, storeName, rows) {
    const key = this.storeKey(dbName, storeName)
    if (!this.stores.has(key)) this.defineStore(dbName, storeName, 'id')
    const map = this.data.get(key)
    const keyPath = this.stores.get(key).keyPath
    for (const row of rows) {
      const k = row && typeof row === 'object' ? row[keyPath] : undefined
      if (k !== undefined && k !== null) map.set(k, row)
    }
  }

  seedRaw(dbName, storeName, pairs) {
    const key = this.storeKey(dbName, storeName)
    if (!this.stores.has(key)) this.defineStore(dbName, storeName, 'id')
    const map = this.data.get(key)
    for (const [k, value] of pairs) map.set(k, value)
  }

  rows(dbName, storeName) {
    return [...(this.data.get(this.storeKey(dbName, storeName))?.values() || [])]
  }
}

const failures = []
const checks = []
const check = (name, ok, detail = '') => (ok ? checks : failures).push(`${name}${detail ? ` — ${detail}` : ''}`)
const tick = () => new Promise(resolve => setTimeout(resolve, 0))
const unhandled = []
process.on('unhandledRejection', reason => {
  const detail = `rejection: ${reason?.stack || reason?.message || reason}`
  unhandled.push(detail)
  console.error(detail)
})

// Deterministic PRNG: chaotic shapes, reproducible failures.
function mulberry32(seed) {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}
const rnd = mulberry32(0xc0ffee)

function response(status, body, { jsonThrows = false, text } = {}) {
  const raw = text !== undefined ? text : JSON.stringify(body ?? null)
  return {
    status,
    ok: status >= 200 && status < 300,
    json: async () => {
      if (jsonThrows) throw new Error('invalid JSON')
      return body === undefined ? undefined : structuredClone(body)
    },
    text: async () => raw,
  }
}

const db = new FakeIndexedDB()
globalThis.indexedDB = db
globalThis.navigator ||= {}
navigator.onLine = false
globalThis.crypto ||= (await import('node:crypto')).webcrypto
globalThis.localStorage = {
  values: new Map(),
  getItem(k) { return this.values.has(k) ? this.values.get(k) : null },
  setItem(k, v) { this.values.set(k, String(v)) },
  removeItem(k) { this.values.delete(k) },
  clear() { this.values.clear() },
}
globalThis.IDBKeyRange = { only: value => ({ only: value }) }

const apiCalls = []
globalThis.__chaosApi = { impl: async () => response(200, {}) }

const vite = await createServer({
  configFile: false,
  root: process.cwd(),
  logLevel: 'silent',
  appType: 'custom',
  server: { middlewareMode: true },
  plugins: [{
    name: 'stub-relay-api-for-chaos-test',
    enforce: 'pre',
    resolveId(source) {
      if (source.includes('lib/api')) return '\0test-chaos-api'
    },
    load(id) {
      if (id !== '\0test-chaos-api') return
      return `
        export async function apiFetch(path, options) {
          globalThis.__chaosApiCalls.push({ path, options })
          return globalThis.__chaosApi.impl(path, options)
        }
        export async function readError(res, fallback = 'Request failed') {
          const text = await res.text().catch(() => '')
          if (!text) return fallback
          try {
            const data = JSON.parse(text)
            if (typeof data === 'string') return data
            if (data?.error) return data.error
            if (data?.message) return data.message
          } catch {}
          return text
        }
        export function getRelayUrl() { return globalThis.localStorage.getItem('relayUrl') || '' }
        export function getDeviceToken() { return '' }
        export function authHeaders() { return {} }
        export function jsonHeaders() { return { 'Content-Type': 'application/json' } }
      `
    },
  }],
})

const parsedBody = call => {
  try { return JSON.parse(call.options.body) } catch { return null }
}
const callKey = call => {
  const body = parsedBody(call)
  return body?.id || body?.sku || null
}
const idFromBody = raw => {
  try {
    const body = JSON.parse(raw)
    return body?.id || body?.sku || null
  } catch { return null }
}
const respond = async (res) => {
  apiCalls.at(-1).responseStatus = res.status
  return res
}

try {
  globalThis.__chaosApiCalls = apiCalls
  const { useMutationStore } = await vite.ssrLoadModule('/src/stores/mutations.js')
  setActivePinia(createPinia())
  let store = useMutationStore()
  await store.load()

  // ------------------------------------------------------------------
  // 1. 500 rapid offline enqueues: duplicate keys, hostile body shapes.
  // ------------------------------------------------------------------
  const weird = [
    null,
    { id: 'nested', payload: { deeply: { value: '😀漢字' } } },
    { id: 'huge', text: 'x'.repeat(120_000) },
    { id: 'looks-broken', text: '{"unterminated":' },
    { id: 'json-array', text: '[1,2,3' },
    'not-an-object',
    42,
  ]
  const requests = []
  for (let i = 0; i < 500; i++) {
    const duplicate = i < 250
    const key = duplicate ? `dup:${i % 100}` : `uniq:${i}`
    const method = ['POST', 'PUT', 'DELETE', 'PATCH'][Math.floor(rnd() * 4)]
    const body = weird[Math.floor(rnd() * weird.length)]
    const objectBody = body && typeof body === 'object' && !Array.isArray(body) ? { ...body } : { value: body }
    requests.push({
      key,
      method,
      path: i % 17 === 0 ? '/api/chaos/409' : `/api/chaos/${i % 7}`,
      body: { ...objectBody, id: key },
      entity: 'chaos',
      entityId: `e-${i}`,
      idempotencyKey: key,
    })
  }
  const expectedRows = new Set(requests.map(r => r.key)).size
  const enqueueResults = requests.map(r => store.enqueue(r))
  const settled = await Promise.allSettled(enqueueResults)
  await tick()
  const rejectedEnqueues = settled.filter(x => x.status === 'rejected')
  check('500 offline enqueues do not reject', rejectedEnqueues.length === 0, `rejected=${rejectedEnqueues.length}`)
  check('duplicate keys collapse to one pending row', store.items.length === expectedRows,
    `rows=${store.items.length} expected=${expectedRows}`)
  check('all rapid enqueues persisted exactly once', db.rows('tajfroid-mutation-queue', 'mutations').length === expectedRows,
    `persisted=${db.rows('tajfroid-mutation-queue', 'mutations').length}`)
  check('offline burst sends no request', apiCalls.length === 0, `calls=${apiCalls.length}`)
  const counts = store.statusCounts()
  check('status counts are bounded to the three statuses',
    JSON.stringify(Object.keys(counts).sort()) === JSON.stringify(['applied', 'conflict', 'pending']),
    JSON.stringify(counts))
  check('status counts cover every pending mutation',
    counts.pending === expectedRows && counts.conflict === 0 && counts.applied === 0,
    JSON.stringify(counts))
  check('no unhandled rejection during burst', unhandled.length === 0, unhandled.join('; '))

  // ------------------------------------------------------------------
  // 2. Flapping connectivity + network/500/409/invalid-JSON replay.
  // ------------------------------------------------------------------
  let phase = 0
  globalThis.__chaosApi.impl = async (path, options) => {
    if (phase === 0) navigator.onLine = rnd() > 0.22
    const roll = rnd()
    if (path.includes('/409')) {
      if (roll < 0.5) return respond(response(409, null, { jsonThrows: true, text: 'Conflict' }))
      if (roll < 0.75) return respond(response(409, 'plain conflict', { text: 'plain conflict' }))
      return respond(response(409, { error: 'stale token', currentUpdatedAt: 'server-v2' }))
    }
    if (phase === 0 && roll < 0.14) { apiCalls.at(-1).responseStatus = 0; throw new Error('simulated network throw') }
    if (phase === 0 && roll < 0.28) return respond(response(500, { error: 'simulated 500' }))
    if (phase === 0 && roll < 0.38) return respond(response(200, undefined, { jsonThrows: true }))
    return respond(response(201, { id: idFromBody(options.body) || 'relay-generated' }))
  }
  for (let round = 0; round < 120; round++) {
    navigator.onLine = true
    await store.flush()
    if (round > 6 && store.statusCounts().pending === 0) break
  }
  phase = 1
  for (let round = 0; round < 120; round++) {
    navigator.onLine = true
    await store.flush()
    if (store.statusCounts().pending === 0) break
  }
  check('flapping flush drains every pending mutation', store.statusCounts().pending === 0,
    JSON.stringify(store.statusCounts()))
  const appliedKeys = new Set(store.items.filter(x => x.status === 'applied').map(x => x.idempotencyKey))
  const successByKey = new Map()
  for (const call of apiCalls) {
    if (call.responseStatus >= 200 && call.responseStatus < 300) {
      const k = callKey(call)
      if (k) successByKey.set(k, (successByKey.get(k) || 0) + 1)
    }
  }
  const duplicatedCreates = [...appliedKeys].filter(k => k && successByKey.get(k) > 1)
  check('no applied create was POSTed twice', duplicatedCreates.length === 0,
    duplicatedCreates.slice(0, 3).join(','))

  // Dedicated malformed-409 probe (previously threw on null JSON bodies).
  for (const x of [...store.items]) {
    if (x?.status === 'conflict') await store.remove(x.id)
  }
  const malformedResponses = [
    response(409, null, { jsonThrows: true, text: 'Conflict' }),
    response(409, undefined, { jsonThrows: true, text: 'not json' }),
    response(409, 'bare string'),
    response(409, [1, 2, 3]),
  ]
  const probeIds = []
  navigator.onLine = false
  for (let i = 0; i < malformedResponses.length; i++) {
    probeIds.push(await store.enqueue({
      method: 'PUT', path: '/api/chaos/409', body: { id: `probe-409:${i}`, note: 'malformed body' },
      entity: 'chaos', entityId: `probe-409:${i}`, idempotencyKey: `probe409:${i}`,
    }))
  }
  let probeIndex = 0
  globalThis.__chaosApi.impl = async () => respond(malformedResponses[probeIndex++] || response(200, {}))
  navigator.onLine = true
  await store.flush()
  const probes = probeIds.map(id => store.items.find(x => x.id === id))
  check('malformed 409 bodies become conflicts, not crashes',
    probes.every(x => x?.status === 'conflict' && typeof x.lastError === 'string'),
    probes.map(x => x?.status).join(','))
  check('malformed 409 probes recorded no unhandled rejection', unhandled.length === 0, unhandled.join('; '))

  // ------------------------------------------------------------------
  // 3. Reload the store while a flush is in flight.
  // ------------------------------------------------------------------
  for (const id of probeIds) await store.remove(id)
  const reloadIds = []
  navigator.onLine = false
  for (let i = 0; i < 12; i++) {
    reloadIds.push(await store.enqueue({
      method: 'POST', path: `/api/chaos/reload/${i}`, body: { id: `reload:${i}`, value: i },
      entity: 'chaos', entityId: `reload:${i}`, idempotencyKey: `reload:${i}`,
    }))
  }
  globalThis.__chaosApi.impl = async (path, options) => {
    await tick()
    return respond(response(201, { id: idFromBody(options.body) || 'ok' }))
  }
  const baselineCalls = apiCalls.length
  navigator.onLine = true
  const inFlight = store.flush()
  await tick()
  setActivePinia(createPinia())
  const reloaded = useMutationStore()
  await reloaded.load()
  check('mid-flush reload keeps every queued mutation',
    reloadIds.every(id => reloaded.items.some(x => x.id === id)),
    `found=${reloadIds.filter(id => reloaded.items.some(x => x.id === id)).length}/${reloadIds.length}`)
  check('mid-flush reload keeps pending items pending or applied',
    reloaded.items.filter(x => reloadIds.includes(x.id)).every(x => x.status === 'pending' || x.status === 'applied'))
  const secondFlush = reloaded.flush()
  await Promise.all([inFlight, secondFlush])
  await reloaded.load()
  const reloadKeys = new Set(reloadIds.map((_, i) => `reload:${i}`))
  const reloadCallsForIds = apiCalls.slice(baselineCalls).filter(c => reloadKeys.has(callKey(c)))
  check('concurrent reload flush sends each mutation once', reloadCallsForIds.length === reloadIds.length,
    `calls=${reloadCallsForIds.length} expected=${reloadIds.length}`)
  check('reloaded store observes all applied rows',
    reloadIds.every(id => reloaded.items.some(x => x.id === id && x.status === 'applied')))
  store = reloaded

  // ------------------------------------------------------------------
  // 4. IndexedDB write failures: no lost pending item, no rejection.
  // ------------------------------------------------------------------
  db.failWrites = true
  navigator.onLine = false
  const failedWriteId = await store.enqueue({
    method: 'POST', path: '/api/chaos/write-failure', body: { id: 'write-failure', note: 'survive' },
    entity: 'chaos', entityId: 'write-failure', idempotencyKey: 'write-failure:1',
  })
  check('enqueue survives an IndexedDB write failure',
    store.items.some(x => x.id === failedWriteId && x.status === 'pending'),
    `in-memory=${store.items.some(x => x.id === failedWriteId)}`)
  globalThis.__chaosApi.impl = async (path, options) => respond(response(201, { id: idFromBody(options.body) || 'ok' }))
  navigator.onLine = true
  await store.flush()
  check('flush during write failure keeps in-memory mutation applied',
    store.items.some(x => x.id === failedWriteId && x.status === 'applied'))
  check('write failures raise no unhandled rejection', unhandled.length === 0, unhandled.join('; '))
  db.failWrites = false
  await store.flush()
  const persistedFailure = db.rows('tajfroid-mutation-queue', 'mutations').find(x => x?.id === failedWriteId)
  check('mutation is persisted once writes recover', persistedFailure?.status === 'applied',
    `status=${persistedFailure?.status}`)

  // ------------------------------------------------------------------
  // 5. Conflict storm on one record, then a retry with the current base.
  // ------------------------------------------------------------------
  for (const row of db.rows('tajfroid-mutation-queue', 'mutations')) {
    if (row?.idempotencyKey === 'storm:1') await store.remove(row.id)
  }
  const stormIds = []
  for (let i = 0; i < 20; i++) {
    stormIds.push(await store.recordConflict({
      method: 'PUT', path: '/api/customers/manage?id=storm:1',
      body: { id: 'storm:1', name: `edit-${i}` },
      entity: 'customer', entityId: 'storm:1', baseUpdatedAt: 'server-v8',
      idempotencyKey: 'storm:1', conflict: { error: 'stale token' }, error: 'stale token',
    }))
  }
  check('20 conflicts on one record are all retained',
    stormIds.every(id => store.items.some(x => x.id === id && x.status === 'conflict')),
    `count=${store.items.filter(x => x.idempotencyKey === 'storm:1').length} dbRows=${db.rows('tajfroid-mutation-queue', 'mutations').filter(x => x?.idempotencyKey === 'storm:1').length}`)
  const lastStormId = stormIds.at(-1)
  await store.updateBody(lastStormId, { id: 'storm:1', name: 'resolved' })
  check('storm siblings pruned by updateBody',
    db.rows('tajfroid-mutation-queue', 'mutations').filter(x => x?.idempotencyKey === 'storm:1').length === 1,
    `keep=${lastStormId.slice(0, 8)} rows=${db.rows('tajfroid-mutation-queue', 'mutations').filter(x => x?.idempotencyKey === 'storm:1').map(x => `${x.id.slice(0, 8)}:${x.status}`).join(',')}`)
  navigator.onLine = false
  await store.retry(lastStormId)
  check('storm resolution is pending before replay',
    store.items.some(x => x.id === lastStormId && x.status === 'pending'),
    `status=${store.items.find(x => x.id === lastStormId)?.status}`)
  const stormBaseline = apiCalls.length
  navigator.onLine = true
  globalThis.__chaosApi.impl = async () => respond(response(200, { id: 'storm:1' }))
  await store.flush()
  const stormCalls = apiCalls.slice(stormBaseline).filter(c => parsedBody(c)?.id === 'storm:1')
  check('conflict storm retry sends exactly one write', stormCalls.length === 1,
    `calls=${stormCalls.length} dbRows=${db.rows('tajfroid-mutation-queue', 'mutations').filter(x => x?.idempotencyKey === 'storm:1').map(x => x.status).join(',')}`)
  check('retry uses the current base token',
    stormCalls[0] && parsedBody(stormCalls[0])?.baseUpdatedAt === 'server-v8',
    JSON.stringify(stormCalls[0] && parsedBody(stormCalls[0])))
  check('retry preserves the resolved body',
    stormCalls[0] && parsedBody(stormCalls[0])?.name === 'resolved')
  const stormAfter = db.rows('tajfroid-mutation-queue', 'mutations')
    .filter(x => x?.idempotencyKey === 'storm:1')
  check('no stale conflict rows remain after resolution',
    stormAfter.length <= 1 && stormAfter.every(x => x.status === 'applied'),
    `rows=${stormAfter.map(x => x.status).join(',')}`)

  // Applied replay must not resend a POST, even after a duplicate enqueue.
  const appliedPostItem = store.items.find(x => x?.method === 'POST' && x.status === 'applied' && x.idempotencyKey)
  const appliedPostKey = appliedPostItem?.idempotencyKey || 'replay:1'
  const replayBaseline = apiCalls.length
  navigator.onLine = false
  await store.enqueue({
    method: 'POST', path: '/api/chaos/replay', body: { id: appliedPostKey },
    entity: 'chaos', entityId: appliedPostKey, idempotencyKey: appliedPostKey,
  })
  navigator.onLine = true
  await store.flush()
  const replayCalls = apiCalls.slice(replayBaseline).filter(c => callKey(c) === appliedPostKey)
  check('applied POST replay is not duplicated', replayCalls.length === 0,
    `calls=${replayCalls.length} key=${appliedPostKey}`)

  // ------------------------------------------------------------------
  // 6. Scan queue: large split payloads, corrupt rows, safe pruning.
  // ------------------------------------------------------------------
  localStorage.setItem('relayUrl', 'http://relay.test')
  const SCAN_DB = 'tajfroid-scan-queue'
  db.defineStore(SCAN_DB, 'scans', 'clientId', { status: 'status', updatedAt: 'updatedAt' })
  db.defineStore(SCAN_DB, 'scanPayloads', 'clientId')
  const hugePayload = 'A'.repeat(320_000)
  const terminal = []
  for (let i = 0; i < 125; i++) {
    terminal.push({
      clientId: `term-${i}`, status: 'uploaded', capturedAt: 1000 + i,
      updatedAt: i < 20 ? 1 : 9_000_000, message: 'done',
    })
  }
  const pendingScans = [
    { clientId: 'pending-huge', status: 'pending_upload', capturedAt: 5000, updatedAt: 5000, attempts: 0, sizeBytes: hugePayload.length },
    { clientId: 'pending-retry', status: 'upload_failed', capturedAt: 5001, updatedAt: 5001, attempts: 3, sizeBytes: 8 },
    { clientId: 'pending-nopayload', status: 'pending_upload', capturedAt: 5002, updatedAt: 5002, attempts: 0 },
  ]
  db.seed(SCAN_DB, 'scans', [
    ...terminal,
    ...pendingScans,
    { clientId: 'stopped', status: 'upload_failed', capturedAt: 4000, updatedAt: 4000, attempts: 25 },
    { clientId: 'corrupt-missing-fields', status: 'pending_upload' },
  ])
  db.seedRaw(SCAN_DB, 'scans', [['__corrupt_null', null], ['__corrupt_num', 42]])
  db.seed(SCAN_DB, 'scanPayloads', [
    { clientId: 'pending-huge', ciphertextB64: hugePayload },
    { clientId: 'pending-retry', ciphertextB64: 'QUJDRA==' },
    { clientId: 'stopped', ciphertextB64: 'U1RPUFBFRA==' },
  ])
  globalThis.__chaosApi.impl = async path => respond(path.includes('since=0')
    ? response(200, { scans: [] })
    : response(200, { status: 'uploaded', scanId: 'remote-1' }))
  const { useQueueStore } = await vite.ssrLoadModule('/src/stores/queue.js')
  setActivePinia(createPinia())
  const scans = useQueueStore()
  await scans.load()
  check('scan queue loads corrupt rows without crashing', scans.items.length > 0)
  check('scan metadata never carries ciphertext',
    scans.items.every(x => x == null || typeof x !== 'object' || x.ciphertextB64 === undefined))
  const terminalStatuses = ['uploaded', 'imported', 'rejected', 'failed', 'failed_local']
  const terminalAfterLoad = scans.items.filter(x => x && terminalStatuses.includes(x.status))
  check('terminal scan history is bounded to 100 rows', terminalAfterLoad.length <= 100,
    `terminal=${terminalAfterLoad.length}`)
  check('pending scans survive terminal pruning',
    pendingScans.every(x => db.rows(SCAN_DB, 'scans').some(row => row?.clientId === x.clientId)),
    ['pending-huge', 'pending-retry', 'pending-nopayload', 'stopped'].map(id =>
      `${id}:${db.rows(SCAN_DB, 'scans').find(x => x?.clientId === id)?.status || 'missing'}`).join(' '))
  const scanBaseline = apiCalls.length
  await scans.flush()
  const uploadCalls = apiCalls.slice(scanBaseline).filter(c => c.path === '/api/scans' && c.options.method === 'POST')
  const hugeCall = uploadCalls.find(c => parsedBody(c)?.clientId === 'pending-huge')
  check('large split payload is sent from payload store',
    !!hugeCall && parsedBody(hugeCall)?.ciphertextB64?.length === hugePayload.length,
    `uploads=${uploadCalls.map(c => parsedBody(c)?.clientId).join(',')}`)
  check('successful scan upload drops local ciphertext',
    !db.rows(SCAN_DB, 'scanPayloads').some(x => x?.clientId === 'pending-huge'))
  const stoppedRow = db.rows(SCAN_DB, 'scans').find(x => x?.clientId === 'stopped')
  check('capped upload_failed scan is failed and payload dropped',
    stoppedRow?.status === 'failed' && !db.rows(SCAN_DB, 'scanPayloads').some(x => x?.clientId === 'stopped'),
    `status=${stoppedRow?.status}`)
  check('scan chaos raised no unhandled rejection', unhandled.length === 0, unhandled.join('; '))
} catch (error) {
  check('chaos scenarios completed', false, error?.stack || String(error))
} finally {
  await vite.close()
}

console.log(`PASS ${checks.length}`)
for (const c of checks) console.log(`  ok   ${c}`)
if (failures.length || unhandled.length) {
  console.log(`\nFAIL ${failures.length + unhandled.length}`)
  for (const f of failures) console.log(`  FAIL ${f}`)
  for (const u of unhandled) console.log(`  FAIL ${u}`)
  process.exit(1)
}
