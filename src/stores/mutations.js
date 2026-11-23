import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch, readError } from '../lib/api'

const DB_NAME = 'tajfroid-mutation-queue'
const DB_VERSION = 1
const STORE = 'mutations'
// Applied rows are kept only as a local replay guard. The server is already
// idempotent by client id, so pruning old rows cannot duplicate data.
const MAX_APPLIED = 200
const APPLIED_TTL_MS = 7 * 24 * 60 * 60 * 1000

function openDb() {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION)
    req.onupgradeneeded = () => {
      const db = req.result
      if (!db.objectStoreNames.contains(STORE)) {
        const os = db.createObjectStore(STORE, { keyPath: 'id' })
        os.createIndex('status', 'status', { unique: false })
        os.createIndex('createdAt', 'createdAt', { unique: false })
      }
    }
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
}

async function idbAll() {
  const db = await openDb()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE, 'readonly')
    const req = tx.objectStore(STORE).getAll()
    req.onsuccess = () => resolve(req.result || [])
    req.onerror = () => reject(req.error)
  })
}

async function idbPut(item) {
  const db = await openDb()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE, 'readwrite')
    tx.objectStore(STORE).put(item)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
}

async function idbDel(id) {
  const db = await openDb()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE, 'readwrite')
    tx.objectStore(STORE).delete(id)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
}

let seqCounter = 0
function nextSeq() { return ++seqCounter }

export const useMutationStore = defineStore('mutations', () => {
  const items = ref([])
  // O(1) lookups for replace/retry/idempotency instead of scanning the queue
  // once per flushed item (which made replay O(n^2)).
  const index = new Map()
  const byKey = new Map()
  const syncing = ref(false)
  const lastError = ref('')

  function reindex() {
    index.clear(); byKey.clear()
    items.value.forEach((x, i) => {
      index.set(x.id, i)
      if (x.idempotencyKey &&
          ['applied', 'pending', 'conflict'].includes(x.status) &&
          !byKey.has(x.idempotencyKey)) {
        byKey.set(x.idempotencyKey, x.id)
      }
    })
  }

  async function load() {
    const all = await idbAll()
    // seqCounter resets on page reload; persisted seq values do not. Seed it
    // from storage so newly queued mutations always sort after old ones.
    for (const x of all) seqCounter = Math.max(seqCounter, Number(x.seq) || 0)
    // Replay-safe ordered processing: sequential by seq (assigned on enqueue/recordConflict).
    all.sort((a, b) => (a.seq || 0) - (b.seq || 0))
    items.value = all
    reindex()
    await pruneApplied()
  }

  async function pruneApplied() {
    const now = Date.now()
    const applied = items.value
      .filter(x => x.status === 'applied')
      .sort((a, b) => (b.updatedAt || b.createdAt || 0) - (a.updatedAt || a.createdAt || 0))
    const stale = new Set()
    applied.forEach((x, i) => {
      const age = now - (x.updatedAt || x.createdAt || 0)
      if (i >= MAX_APPLIED || age > APPLIED_TTL_MS) stale.add(x.id)
    })
    if (!stale.size) return
    for (const id of stale) await idbDel(id)
    items.value = items.value.filter(x => !stale.has(x.id))
    reindex()
  }

  async function enqueue({ method, path, body, entity, entityId, baseUpdatedAt, idempotencyKey }) {
    const key = idempotencyKey || (body && typeof body === 'object' ? body.id : null)
    const existing = key ? byKey.get(key) : null
    if (existing) return existing // idempotent replay: already tracked
    const item = {
      id: crypto.randomUUID(),
      seq: nextSeq(),
      idempotencyKey: key || null,
      method, path, body: body ?? null,
      entity: entity || null, entityId: entityId || null,
      baseUpdatedAt: baseUpdatedAt || null,
      status: 'pending', attempts: 0, lastError: null, serverId: null,
      createdAt: Date.now(), updatedAt: Date.now(),
    }
    await idbPut(item)
    index.set(item.id, items.value.push(item) - 1)
    if (key) byKey.set(key, item.id)
    flush().catch(() => {})
    return item.id
  }

  async function flush() {
    if (syncing.value) return
    if (!navigator.onLine) return
    syncing.value = true
    lastError.value = ''
    try {
      const all = await idbAll()
      // Replay-safe ordering: process in seq order; applied items stay stored for idempotent replay.
      const pending = all.filter(x => x.status === 'pending' || x.status === 'conflict').sort((a, b) => (a.seq || 0) - (b.seq || 0))
      for (const item of pending) {
        try {
          const headers = { 'Content-Type': 'application/json' }
          // Optimistic concurrency: send baseUpdatedAt if present.
          const payload = item.body && typeof item.body === 'object' ? { ...item.body } : item.body
          if (payload && item.baseUpdatedAt) payload.baseUpdatedAt = item.baseUpdatedAt
          // Idempotent replay: skip POST creates whose idempotencyKey already applied/conflict with a serverId.
          if (item.method === 'POST' && item.idempotencyKey && item.serverId) {
            continue
          }
          const res = await apiFetch(item.path, {
            method: item.method,
            headers,
            body: payload ? JSON.stringify(payload) : undefined,
          })
          if (res.status === 409) {
            const body409 = await res.json().catch(() => ({}))
            const msg = body409.error || 'Conflict'
            const next = { ...item, status: 'conflict', lastError: msg, conflict: body409, attempts: (item.attempts||0)+1, updatedAt: Date.now() }
            await idbPut(next); replace(next)
            lastError.value = msg
            continue
          }
          if (!res.ok) throw new Error(await readError(res, `HTTP ${res.status}`))
          const data = await res.json().catch(() => null)
          const serverId = (data && (data.id || data.sku)) || item.serverId || null
          const next = { ...item, status: 'applied', serverId, lastError: null, updatedAt: Date.now() }
          await idbPut(next); replace(next)
        } catch (e) {
          const next = { ...item, status: 'pending', lastError: e.message || String(e), attempts: (item.attempts||0)+1, updatedAt: Date.now() }
          await idbPut(next); replace(next)
          lastError.value = next.lastError
          if (!navigator.onLine) break
        }
      }
      await pruneApplied()
    } finally {
      syncing.value = false
    }
  }

  function replace(item) {
    const i = index.get(item.id)
    if (i !== undefined) items.value[i] = item
  }

  async function remove(id) {
    await idbDel(id)
    const i = index.get(id)
    const key = i !== undefined ? items.value[i].idempotencyKey : null
    if (key && byKey.get(key) === id) byKey.delete(key)
    items.value = items.value.filter(x => x.id !== id)
    reindex()
  }

  async function retry(id) {
    const i = index.get(id)
    const item = i !== undefined ? items.value[i] : null
    if (!item) return
    const next = { ...item, status: 'pending', lastError: null, updatedAt: Date.now() }
    await idbPut(next); replace(next)
    flush().catch(() => {})
  }

  async function updateBody(id, body) {
    const i = index.get(id)
    const item = i !== undefined ? items.value[i] : null
    if (!item) return
    const next = { ...item, body, status: 'pending', lastError: null, updatedAt: Date.now() }
    await idbPut(next); replace(next)
  }

  async function recordConflict({ method, path, body, entity, entityId, baseUpdatedAt, conflict, error }) {
    const key = (body && typeof body === 'object' ? body.id : null) || null
    const item = {
      id: crypto.randomUUID(), seq: nextSeq(), idempotencyKey: key,
      method, path, body: body ?? null,
      entity: entity || null, entityId: entityId || null,
      baseUpdatedAt: baseUpdatedAt || null,
      status: 'conflict', attempts: 0,
      lastError: error || conflict?.error || 'Conflict',
      conflict: conflict || null, serverId: null,
      createdAt: Date.now(), updatedAt: Date.now(),
    }
    await idbPut(item)
    index.set(item.id, items.value.push(item) - 1)
    if (key) byKey.set(key, item.id)
    return item.id
  }

  async function conflictFromResponse(res, { method, path, body, entity, entityId, baseUpdatedAt, fallback }) {
    const d = await res.json().catch(() => ({}))
    await recordConflict({ method, path, body, entity, entityId, baseUpdatedAt, conflict: d, error: d.error })
    const e = new Error(d.error || fallback || 'Conflict')
    e.conflict = true; e.current = d; throw e
  }

  function statusCounts() {
    const c = { pending: 0, conflict: 0, applied: 0 }
    for (const x of items.value) c[x.status] = (c[x.status] || 0) + 1
    return c
  }

  return { items, syncing, lastError, load, enqueue, flush, remove, retry, updateBody, recordConflict, conflictFromResponse, statusCounts }
})
