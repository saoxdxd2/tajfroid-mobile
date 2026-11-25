import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch, readError } from '../lib/api'

const DB_NAME = 'tajfroid-mutation-queue'
const DB_VERSION = 1
const STORE = 'mutations'
const STATUSES = ['pending', 'conflict', 'applied']
// Applied rows are kept only as a local replay guard. The server is already
// idempotent by client id, so pruning old rows cannot duplicate data.
const MAX_APPLIED = 200
const APPLIED_TTL_MS = 7 * 24 * 60 * 60 * 1000
// Keep a bounded tombstone of applied keys so a pruned row cannot cause the
// same create to be POSTed again in the same app lifetime.
const MAX_SETTLED_KEYS = MAX_APPLIED * 4

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
  try {
    return await new Promise((resolve, reject) => {
      const tx = db.transaction(STORE, 'readonly')
      const req = tx.objectStore(STORE).getAll()
      req.onsuccess = () => resolve(req.result || [])
      req.onerror = () => reject(req.error)
    })
  } finally {
    db.close?.()
  }
}

// Items come from a reactive Vue ref; IndexedDB cannot structuredClone Proxy
// values. Persist a plain copy so retries/status transitions never fail.
function plainClone(value) {
  try { return JSON.parse(JSON.stringify(value)) } catch { return { ...value } }
}

async function idbPut(item) {
  const db = await openDb()
  try {
    return await new Promise((resolve, reject) => {
      const tx = db.transaction(STORE, 'readwrite')
      tx.objectStore(STORE).put(plainClone(item))
      tx.oncomplete = () => resolve()
      tx.onerror = () => reject(tx.error)
      tx.onabort = () => reject(tx.error || new Error('IndexedDB write aborted'))
    })
  } finally {
    db.close?.()
  }
}

async function idbDel(id) {
  const db = await openDb()
  try {
    return await new Promise((resolve, reject) => {
      const tx = db.transaction(STORE, 'readwrite')
      tx.objectStore(STORE).delete(id)
      tx.oncomplete = () => resolve()
      tx.onerror = () => reject(tx.error)
      tx.onabort = () => reject(tx.error || new Error('IndexedDB delete aborted'))
    })
  } finally {
    db.close?.()
  }
}

let seqCounter = 0
function nextSeq() { return ++seqCounter }

// State below is module-scoped on purpose: a reload creates a new Pinia store,
// but the in-flight flush, dedupe claims and failed-write recovery must survive
// it (otherwise a replay can POST the same mutation twice).
const views = new Set()
const keyClaims = new Map() // idempotency key -> item id, claimed synchronously
const settledKeys = new Map() // applied idempotency key -> updatedAt tombstone
const unpersisted = new Map() // id -> item whose IndexedDB write failed
let activeFlush = null
let flushAgain = false

function trimMap(map, max) {
  while (map.size > max) map.delete(map.keys().next().value)
}

function normalizeKey(value) {
  return typeof value === 'string' && value ? value : null
}

// Corrupt/legacy rows must never take down the queue. Keep every row that has
// an id, repair missing fields, and drop only rows with no usable identity.
function normalizeRow(raw) {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null
  const id = normalizeKey(raw.id) || (raw.id != null ? String(raw.id) : null)
  if (!id) return null
  const now = Date.now()
  const num = (v, fallback) => (Number.isFinite(Number(v)) ? Number(v) : fallback)
  return {
    ...raw,
    id,
    seq: num(raw.seq, 0),
    idempotencyKey: normalizeKey(raw.idempotencyKey),
    method: typeof raw.method === 'string' ? raw.method : 'GET',
    path: typeof raw.path === 'string' ? raw.path : '/',
    body: raw.body ?? null,
    status: STATUSES.includes(raw.status) ? raw.status : 'pending',
    attempts: num(raw.attempts, 0),
    lastError: typeof raw.lastError === 'string' ? raw.lastError : null,
    conflict: raw.conflict ?? null,
    serverId: raw.serverId ?? null,
    createdAt: num(raw.createdAt, now),
    updatedAt: num(raw.updatedAt, num(raw.createdAt, now)),
  }
}

async function readAll() {
  const rows = await idbAll()
  const byId = new Map()
  for (const row of rows) {
    const item = normalizeRow(row)
    if (item) byId.set(item.id, item)
  }
  // A failed IndexedDB write must not make an accepted mutation vanish.
  for (const item of unpersisted.values()) {
    const normalized = normalizeRow(item)
    if (normalized) byId.set(normalized.id, normalized)
  }
  return [...byId.values()]
}

async function safePut(item) {
  try {
    await idbPut(item)
    unpersisted.delete(item.id)
    return true
  } catch {
    unpersisted.set(item.id, item)
    return false
  }
}

async function safeDel(id) {
  unpersisted.delete(id)
  try {
    await idbDel(id)
    return true
  } catch {
    return false
  }
}

function reindex(view) {
  view.index.clear()
  view.byKey.clear()
  view.items.value.forEach((x, i) => {
    if (!x || typeof x !== 'object') return
    view.index.set(x.id, i)
    const key = x.idempotencyKey
    if (key && STATUSES.includes(x.status) && !view.byKey.has(key)) view.byKey.set(key, x.id)
  })
}

function publish(item) {
  for (const view of views) {
    if (!view.loaded) continue
    const i = view.index.get(item.id)
    if (i === undefined) view.index.set(item.id, view.items.value.push(item) - 1)
    else view.items.value[i] = item
    const key = item.idempotencyKey
    if (!key) continue
    if (STATUSES.includes(item.status) && !view.byKey.has(key)) view.byKey.set(key, item.id)
    else if (view.byKey.get(key) === item.id && !STATUSES.includes(item.status)) view.byKey.delete(key)
  }
}

function forgetEverywhere(id) {
  for (const view of views) {
    if (!view.loaded) continue
    const i = view.index.get(id)
    if (i !== undefined) {
      view.items.value.splice(i, 1)
      reindex(view)
    }
  }
  unpersisted.delete(id)
  for (const [key, owner] of keyClaims) {
    if (owner === id) keyClaims.delete(key)
  }
}

// Synchronous claim: 500 enqueues in one tick must not create 500 rows for 100
// distinct idempotency keys. The claim happens before the first await.
function claimKey(key) {
  if (!key) return null
  const claimed = keyClaims.get(key)
  if (claimed) return claimed
  for (const view of views) {
    const owner = view.loaded ? view.byKey.get(key) : null
    if (owner) {
      keyClaims.set(key, owner)
      return owner
    }
  }
  // The applied row may already have been pruned; the tombstone still proves
  // this create was accepted. Return a stable, non-persisted id.
  if (settledKeys.has(key)) return `settled:${key}`
  return null
}

function markSettled(item) {
  if (!item.idempotencyKey) return
  settledKeys.set(item.idempotencyKey, item.updatedAt || Date.now())
  keyClaims.set(item.idempotencyKey, item.id)
  trimMap(settledKeys, MAX_SETTLED_KEYS)
  trimMap(keyClaims, MAX_SETTLED_KEYS * 2)
}

async function conflictBody(res) {
  let raw = null
  try { raw = await res.json() } catch { raw = null }
  if (raw == null) return { raw, error: 'Conflict' }
  if (typeof raw === 'string') return { raw, error: raw || 'Conflict' }
  if (typeof raw === 'object') {
    const error = (typeof raw.error === 'string' && raw.error) ||
      (typeof raw.message === 'string' && raw.message) || 'Conflict'
    return { raw, error }
  }
  return { raw, error: 'Conflict' }
}

async function pruneApplied(view) {
  const now = Date.now()
  const applied = view.items.value
    .filter(x => x && x.status === 'applied')
    .sort((a, b) => (b.updatedAt || b.createdAt || 0) - (a.updatedAt || a.createdAt || 0))
  const stale = applied.filter((x, i) =>
    i >= MAX_APPLIED || now - (x.updatedAt || x.createdAt || 0) > APPLIED_TTL_MS)
  for (const candidate of stale) {
    // Re-check the live view: a retry/updateBody during the awaited delete
    // must never turn this into a lost pending mutation.
    const current = view.items.value.find(x => x?.id === candidate.id)
    if (!current || current.status !== 'applied') continue
    if (await safeDel(candidate.id)) forgetEverywhere(candidate.id)
  }
}

async function flushOnce(view) {
  // A mutation accepted during an IndexedDB outage lives only in memory until
  // writes recover; persist it before deciding what still needs replaying.
  if (unpersisted.size) {
    for (const item of [...unpersisted.values()]) await safePut(item)
  }
  const all = await readAll()
  const byId = new Map(all.map(x => [x.id, x]))
  for (const x of all) {
    if (x.status === 'applied' && x.idempotencyKey) {
      settledKeys.set(x.idempotencyKey, x.updatedAt || Date.now())
    }
  }
  trimMap(settledKeys, MAX_SETTLED_KEYS)

  const pending = all
    .filter(x => x.status === 'pending' || x.status === 'conflict')
    .sort((a, b) => (a.seq || 0) - (b.seq || 0))

  for (const queued of pending) {
    if (!navigator.onLine) break
    const item = byId.get(queued.id) || queued
    if (item.status === 'applied') continue
    // A create whose key is already applied (including pruned rows) must not
    // be POSTed again. Conflict rows stay retryable: a 409 means the server
    // did not apply the write.
    if (item.method === 'POST' && item.idempotencyKey && settledKeys.has(item.idempotencyKey)) continue

    try {
      const headers = { 'Content-Type': 'application/json' }
      const payload = item.body && typeof item.body === 'object' ? { ...item.body } : item.body
      if (payload && typeof payload === 'object' && item.baseUpdatedAt) payload.baseUpdatedAt = item.baseUpdatedAt
      const res = await apiFetch(item.path, {
        method: item.method,
        headers,
        body: payload ? JSON.stringify(payload) : undefined,
      })
      if (res.status === 409) {
        const detail = await conflictBody(res)
        const next = {
          ...item,
          status: 'conflict',
          lastError: detail.error,
          conflict: detail.raw,
          attempts: (item.attempts || 0) + 1,
          updatedAt: Date.now(),
        }
        await safePut(next); publish(next); byId.set(next.id, next)
        view.lastError.value = detail.error
        continue
      }
      if (!res.ok) throw new Error(await readError(res, `HTTP ${res.status}`))
      // 2xx means accepted even when the response body is invalid JSON.
      const data = await res.json().catch(() => null)
      const serverId = (data && typeof data === 'object' && (data.id || data.sku)) || item.serverId || null
      const next = { ...item, status: 'applied', serverId, lastError: null, updatedAt: Date.now() }
      await safePut(next); publish(next); byId.set(next.id, next)
      markSettled(next)
    } catch (e) {
      const next = {
        ...item,
        status: 'pending',
        lastError: e?.message || String(e),
        attempts: (item.attempts || 0) + 1,
        updatedAt: Date.now(),
      }
      await safePut(next); publish(next); byId.set(next.id, next)
      view.lastError.value = next.lastError
      if (!navigator.onLine) break
    }
  }
  await pruneApplied(view)
}

const activeTimers = new Set()
let retryTimer = null
const RETRY_BASE_MS = 5000
const RETRY_MAX_MS = 300000

function computeBackoffMs(attempts) {
  const step = Math.min(RETRY_MAX_MS, RETRY_BASE_MS * Math.pow(3, Math.max(0, attempts - 1)))
  const jitter = Math.floor(step * 0.2 * Math.random())
  return step + jitter
}

export const useMutationStore = defineStore('mutations', () => {
  const items = ref([])
  const index = new Map()
  const byKey = new Map()
  const syncing = ref(false)
  const lastError = ref('')
  const nextRetryAt = ref(0)
  const view = { items, index, byKey, lastError, syncing, loaded: false }
  views.add(view)

  function scheduleNextRetry() {
    if (typeof window === 'undefined') return
    if (retryTimer) { clearTimeout(retryTimer); retryTimer = null }
    const pendingItems = items.value.filter(x => x && x.status === 'pending')
    if (!pendingItems.length) { nextRetryAt.value = 0; return }
    const maxAttempts = Math.max(1, ...pendingItems.map(x => Number(x.attempts) || 1))
    const delayMs = computeBackoffMs(maxAttempts)
    nextRetryAt.value = Date.now() + delayMs
    retryTimer = setTimeout(() => {
      retryTimer = null
      if (typeof navigator === 'undefined' || navigator.onLine) {
        requestFlush()
      }
    }, delayMs)
  }

  function requestFlush() {
    if (activeFlush) { flushAgain = true; return }
    flush().catch(() => {})
  }

  async function load() {
    const all = await readAll()
    for (const x of all) seqCounter = Math.max(seqCounter, x.seq || 0)
    all.sort((a, b) => (a.seq || 0) - (b.seq || 0))
    items.value = all
    reindex(view)
    view.loaded = true
    for (const x of all) {
      if (x.status === 'applied' && x.idempotencyKey) {
        settledKeys.set(x.idempotencyKey, x.updatedAt || Date.now())
      }
      if (x.idempotencyKey && !keyClaims.has(x.idempotencyKey)) keyClaims.set(x.idempotencyKey, x.id)
    }
    trimMap(settledKeys, MAX_SETTLED_KEYS)
    trimMap(keyClaims, MAX_SETTLED_KEYS * 2)
    await pruneApplied(view)
  }

  async function enqueue({ method, path, body, entity, entityId, baseUpdatedAt, idempotencyKey }) {
    const key = normalizeKey(idempotencyKey) ||
      normalizeKey(body && typeof body === 'object' ? body.id : null)
    const existing = claimKey(key)
    if (existing) return existing
    const item = {
      id: crypto.randomUUID(),
      seq: nextSeq(),
      idempotencyKey: key,
      method, path, body: body ?? null,
      entity: entity || null, entityId: entityId || null,
      baseUpdatedAt: baseUpdatedAt || null,
      status: 'pending', attempts: 0, lastError: null, serverId: null,
      conflict: null,
      createdAt: Date.now(), updatedAt: Date.now(),
    }
    if (key) keyClaims.set(key, item.id)
    publish(item)
    await safePut(item)
    requestFlush()
    return item.id
  }

  async function flush() {
    if (!navigator.onLine) return
    if (activeFlush) {
      syncing.value = true
      try { await activeFlush } finally { syncing.value = false }
      return
    }
    let release
    activeFlush = new Promise(resolve => { release = resolve })
    syncing.value = true
    lastError.value = ''
    try {
      do {
        flushAgain = false
        await flushOnce(view)
      } while (flushAgain && navigator.onLine)
    } finally {
      syncing.value = false
      activeFlush = null
      release()
      scheduleNextRetry()
    }
  }

  async function remove(id) {
    if (await safeDel(id)) forgetEverywhere(id)
  }

  // Once the user resolves a conflict, older conflicts for that record are
  // superseded and must not be replayed alongside the resolution.
  async function supersedeConflicts(item, keepId) {
    if (item.status !== 'conflict' || !item.idempotencyKey) return
    const siblings = items.value
      .filter(x => x && x.id !== keepId && x.idempotencyKey === item.idempotencyKey && x.status === 'conflict')
      .map(x => x.id)
    for (const sibling of siblings) {
      if (await safeDel(sibling)) forgetEverywhere(sibling)
    }
  }

  async function retry(id) {
    const i = index.get(id)
    const item = i !== undefined ? items.value[i] : null
    if (!item) return
    const next = { ...item, status: 'pending', lastError: null, updatedAt: Date.now() }
    await safePut(next); publish(next)
    if (item.idempotencyKey) {
      settledKeys.delete(item.idempotencyKey)
      keyClaims.set(item.idempotencyKey, id)
    }
    await supersedeConflicts(item, id)
    requestFlush()
  }

  async function updateBody(id, body) {
    const i = index.get(id)
    const item = i !== undefined ? items.value[i] : null
    if (!item) return
    const next = { ...item, body, status: 'pending', lastError: null, updatedAt: Date.now() }
    await safePut(next); publish(next)
    if (next.idempotencyKey) {
      settledKeys.delete(next.idempotencyKey)
      keyClaims.set(next.idempotencyKey, id)
    }
    await supersedeConflicts(item, id)
  }

  async function recordConflict({ method, path, body, entity, entityId, baseUpdatedAt, conflict, error, idempotencyKey }) {
    const key = normalizeKey(idempotencyKey) ||
      normalizeKey(body && typeof body === 'object' ? body.id : null)
    const item = {
      id: crypto.randomUUID(), seq: nextSeq(), idempotencyKey: key,
      method, path, body: body ?? null,
      entity: entity || null, entityId: entityId || null,
      baseUpdatedAt: baseUpdatedAt || null,
      status: 'conflict', attempts: 0,
      lastError: typeof error === 'string' ? error : 'Conflict',
      conflict: conflict ?? null, serverId: null,
      createdAt: Date.now(), updatedAt: Date.now(),
    }
    await safePut(item)
    publish(item)
    if (key) keyClaims.set(key, item.id)
    return item.id
  }

  async function conflictFromResponse(res, { method, path, body, entity, entityId, baseUpdatedAt, fallback }) {
    const detail = await conflictBody(res)
    await recordConflict({ method, path, body, entity, entityId, baseUpdatedAt, conflict: detail.raw, error: detail.error })
    const e = new Error(detail.error || fallback || 'Conflict')
    e.conflict = true; e.current = detail.raw; throw e
  }

  function statusCounts() {
    const counts = { pending: 0, conflict: 0, applied: 0 }
    for (const x of items.value) {
      if (!x || typeof x !== 'object') continue
      if (Object.prototype.hasOwnProperty.call(counts, x.status)) counts[x.status]++
    }
    return counts
  }

  return { items, syncing, lastError, nextRetryAt, load, enqueue, flush, remove, retry, updateBody, recordConflict, conflictFromResponse, statusCounts }
})
