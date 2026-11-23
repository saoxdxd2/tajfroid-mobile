import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch, readError } from '../lib/api'

const DB_NAME = 'tajfroid-mutation-queue'
const DB_VERSION = 1
const STORE = 'mutations'

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

export const useMutationStore = defineStore('mutations', () => {
  const items = ref([])
  const syncing = ref(false)
  const lastError = ref('')

  async function load() {
    const all = await idbAll()
    all.sort((a, b) => (a.createdAt || 0) - (b.createdAt || 0))
    items.value = all
  }

  async function enqueue({ method, path, body, entity, entityId, baseUpdatedAt }) {
    const item = {
      id: crypto.randomUUID(),
      method, path, body: body ?? null,
      entity: entity || null, entityId: entityId || null,
      baseUpdatedAt: baseUpdatedAt || null,
      status: 'pending', attempts: 0, lastError: null,
      createdAt: Date.now(), updatedAt: Date.now(),
    }
    await idbPut(item)
    items.value.push(item)
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
      const pending = all.filter(x => x.status === 'pending' || x.status === 'conflict')
      for (const item of pending) {
        try {
          const headers = { 'Content-Type': 'application/json' }
          // Optimistic concurrency: send baseUpdatedAt if present.
          const payload = item.body && typeof item.body === 'object' ? { ...item.body } : item.body
          if (payload && item.baseUpdatedAt) payload.baseUpdatedAt = item.baseUpdatedAt
          const res = await apiFetch(item.path, {
            method: item.method,
            headers,
            body: payload ? JSON.stringify(payload) : undefined,
          })
          if (res.status === 409) {
            const msg = await readError(res, 'Conflict')
            const next = { ...item, status: 'conflict', lastError: msg, attempts: (item.attempts||0)+1, updatedAt: Date.now() }
            await idbPut(next); replace(next)
            lastError.value = msg
            continue
          }
          if (!res.ok) throw new Error(await readError(res, `HTTP ${res.status}`))
          await idbDel(item.id)
          items.value = items.value.filter(x => x.id !== item.id)
        } catch (e) {
          const next = { ...item, status: 'pending', lastError: e.message || String(e), attempts: (item.attempts||0)+1, updatedAt: Date.now() }
          await idbPut(next); replace(next)
          lastError.value = next.lastError
          if (!navigator.onLine) break
        }
      }
    } finally {
      syncing.value = false
    }
  }

  function replace(item) {
    const i = items.value.findIndex(x => x.id === item.id)
    if (i >= 0) items.value[i] = item
  }

  async function remove(id) {
    await idbDel(id)
    items.value = items.value.filter(x => x.id !== id)
  }

  async function retry(id) {
    const item = items.value.find(x => x.id === id)
    if (!item) return
    const next = { ...item, status: 'pending', lastError: null, updatedAt: Date.now() }
    await idbPut(next); replace(next)
    flush().catch(() => {})
  }

  return { items, syncing, lastError, load, enqueue, flush, remove, retry }
})
