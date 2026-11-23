import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch, readError, getRelayUrl } from '../lib/api'

const DB_NAME = 'tajfroid-scan-queue'
// v2 moves ciphertext into its own store so ordinary metadata reads never
// deserialize a multi-MB payload (v1 kept ciphertextB64 inline on every row).
const DB_VERSION = 2
const STORE = 'scans'
const PAYLOAD_STORE = 'scanPayloads'
const MAX_PX = 2000
const MAX_BYTES = 2_500_000
const JPEG_QUALITY = 0.82

// Bounded terminal history: enough context in the UI without unbounded growth.
const TERMINAL_STATUSES = ['uploaded', 'imported', 'rejected', 'failed', 'failed_local']
const MAX_TERMINAL_ROWS = 100
const TERMINAL_TTL_MS = 7 * 24 * 60 * 60 * 1000
// upload_failed stays retryable, but a permanently failing scan must not keep
// its ciphertext forever. Both limits convert it to a terminal `failed` row.
const MAX_UPLOAD_ATTEMPTS = 20
const UPLOAD_FAILED_TTL_MS = 7 * 24 * 60 * 60 * 1000

function openDb() {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION)
    let migrationError = null
    req.onupgradeneeded = () => {
      const db = req.result
      const tx = req.transaction
      let scans
      if (!db.objectStoreNames.contains(STORE)) {
        scans = db.createObjectStore(STORE, { keyPath: 'clientId' })
        scans.createIndex('status', 'status', { unique: false })
        scans.createIndex('updatedAt', 'updatedAt', { unique: false })
      } else {
        scans = tx.objectStore(STORE)
      }
      if (!db.objectStoreNames.contains(PAYLOAD_STORE)) {
        const payloads = db.createObjectStore(PAYLOAD_STORE, { keyPath: 'clientId' })
        // One-time v1 migration: pull inline ciphertext out of scans.
        const cursorReq = scans.openCursor()
        cursorReq.onsuccess = () => {
          const cursor = cursorReq.result
          if (!cursor) return
          const item = cursor.value
          if (item && item.ciphertextB64) {
            payloads.put({ clientId: item.clientId, ciphertextB64: item.ciphertextB64 })
            delete item.ciphertextB64
            cursor.update(item)
          }
          cursor.continue()
        }
        cursorReq.onerror = () => { migrationError = cursorReq.error || new Error('Queue migration failed') }
      }
    }
    req.onsuccess = () => {
      if (migrationError) {
        req.result.close()
        reject(migrationError)
        return
      }
      resolve(req.result)
    }
    req.onerror = () => reject(req.error || new Error('IndexedDB open failed'))
  })
}

function txDone(tx) {
  return new Promise((resolve, reject) => {
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
    tx.onabort = () => reject(tx.error || new Error('IndexedDB transaction aborted'))
  })
}

// Metadata only: scans rows no longer carry ciphertextB64 (v2 schema).
async function idbGetAll() {
  const db = await openDb()
  try {
    const tx = db.transaction(STORE, 'readonly')
    const done = txDone(tx)
    const req = tx.objectStore(STORE).getAll()
    let rows = []
    req.onsuccess = () => { rows = req.result || [] }
    await done
    return rows
  } finally {
    db.close()
  }
}

// Read only the rows matching the given statuses via the existing status index.
// Callers that need ciphertext fetch it separately, one row at a time.
async function idbGetByStatuses(statuses) {
  const wanted = [...new Set(statuses)].filter(Boolean)
  if (!wanted.length) return []
  const db = await openDb()
  try {
    const tx = db.transaction(STORE, 'readonly')
    const done = txDone(tx)
    const index = tx.objectStore(STORE).index('status')
    const rows = []
    for (const status of wanted) {
      const req = index.getAll(IDBKeyRange.only(status))
      req.onsuccess = () => { if (req.result) rows.push(...req.result) }
    }
    await done
    return rows
  } finally {
    db.close()
  }
}

async function idbGetCiphertext(clientId) {
  const db = await openDb()
  try {
    const tx = db.transaction(PAYLOAD_STORE, 'readonly')
    const done = txDone(tx)
    const req = tx.objectStore(PAYLOAD_STORE).get(clientId)
    let payload
    req.onsuccess = () => { payload = req.result }
    await done
    return payload?.ciphertextB64
  } finally {
    db.close()
  }
}

// Writes metadata and, when supplied, the payload in one transaction. When no
// ciphertext is supplied the existing payload is left untouched, so metadata
// refresh during a status pull cannot break a pending retry.
async function idbPut(item) {
  const db = await openDb()
  try {
    const tx = db.transaction([STORE, PAYLOAD_STORE], 'readwrite')
    const done = txDone(tx)
    const { ciphertextB64, ...meta } = item
    tx.objectStore(STORE).put(meta)
    if (ciphertextB64 !== undefined && ciphertextB64 !== null) {
      tx.objectStore(PAYLOAD_STORE).put({ clientId: item.clientId, ciphertextB64 })
    }
    await done
  } finally {
    db.close()
  }
}

async function idbDeleteCiphertext(clientId) {
  const db = await openDb()
  try {
    const tx = db.transaction(PAYLOAD_STORE, 'readwrite')
    const done = txDone(tx)
    tx.objectStore(PAYLOAD_STORE).delete(clientId)
    await done
  } finally {
    db.close()
  }
}

async function idbDeleteMany(clientIds) {
  if (!clientIds.length) return
  const db = await openDb()
  try {
    const tx = db.transaction([STORE, PAYLOAD_STORE], 'readwrite')
    const done = txDone(tx)
    const scans = tx.objectStore(STORE)
    const payloads = tx.objectStore(PAYLOAD_STORE)
    for (const clientId of clientIds) {
      scans.delete(clientId)
      payloads.delete(clientId)
    }
    await done
  } finally {
    db.close()
  }
}

async function idbDelete(clientId) {
  await idbDeleteMany([clientId])
}

function rowAgeMs(item) {
  const raw = item.updatedAt || item.capturedAt || 0
  return Date.now() - (raw > 1e12 ? raw : raw * 1000)
}

function bytesToBase64(bytes) {
  let binary = ''
  const chunk = 0x8000
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunk))
  }
  return btoa(binary)
}

async function sha256Hex(bytes) {
  const digest = await crypto.subtle.digest('SHA-256', bytes)
  return [...new Uint8Array(digest)].map(b => b.toString(16).padStart(2, '0')).join('')
}

async function downscaleToJpeg(file) {
  const bitmap = await createImageBitmap(file)
  let { width, height } = bitmap
  const scale = Math.min(1, MAX_PX / Math.max(width, height))
  width = Math.max(1, Math.round(width * scale))
  height = Math.max(1, Math.round(height * scale))
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  const ctx = canvas.getContext('2d')
  ctx.drawImage(bitmap, 0, 0, width, height)
  bitmap.close?.()

  let quality = JPEG_QUALITY
  let blob = await new Promise(resolve => canvas.toBlob(resolve, 'image/jpeg', quality))
  while (blob && blob.size > MAX_BYTES && quality > 0.5) {
    quality -= 0.08
    blob = await new Promise(resolve => canvas.toBlob(resolve, 'image/jpeg', quality))
  }
  if (!blob) throw new Error('Could not encode image')
  if (blob.size > MAX_BYTES) throw new Error('Image too large after downscale')
  const buf = await blob.arrayBuffer()
  return new Uint8Array(buf)
}

async function encryptForDesktop(plaintextBytes) {
  const pkB64 = localStorage.getItem('desktopPublicKey') || ''
  if (!pkB64) throw new Error('Desktop public key missing. Pair with the desktop first.')
  const mod = await import('libsodium-wrappers')
  const sodium = mod.default || mod
  if (sodium.ready) await sodium.ready
  const pk = sodium.from_base64(pkB64, sodium.base64_variants.ORIGINAL)
  const cipher = sodium.crypto_box_seal(plaintextBytes, pk)
  return new Uint8Array(cipher)
}

function normalizeStatus(s) {
  if (!s) return 'pending'
  return s
}

export const useQueueStore = defineStore('queue', () => {
  const items = ref([])
  const syncing = ref(false)
  const lastSyncError = ref('')

  async function load() {
    const all = await idbGetAll()
    all.sort((a, b) => (b.capturedAt || 0) - (a.capturedAt || 0))
    items.value = all.map(x => {
      // Defensive: legacy rows (or a failed migration) may still carry it.
      const meta = { ...x }
      delete meta.ciphertextB64
      return meta
    })
    await pruneTerminal()
  }

  // Keep at most MAX_TERMINAL_ROWS terminal rows, dropping older rows past
  // TERMINAL_TTL_MS. Pending / retryable rows are never considered here.
  async function pruneTerminal() {
    const terminal = await idbGetByStatuses(TERMINAL_STATUSES)
    if (!terminal.length) return
    terminal.sort((a, b) => (b.updatedAt || b.capturedAt || 0) - (a.updatedAt || a.capturedAt || 0))
    const stale = terminal.filter((x, i) => i >= MAX_TERMINAL_ROWS || rowAgeMs(x) > TERMINAL_TTL_MS)
    if (!stale.length) return
    const ids = stale.map(x => x.clientId)
    await idbDeleteMany(ids)
    const gone = new Set(ids)
    items.value = items.value.filter(x => !gone.has(x.clientId))
  }

  function upsertLocal(item) {
    const idx = items.value.findIndex(x => x.clientId === item.clientId)
    const safe = { ...item }
    delete safe.ciphertextB64
    if (idx >= 0) items.value[idx] = safe
    else items.value.unshift(safe)
  }

  async function enqueue(file) {
    const clientId = crypto.randomUUID()
    const capturedAt = Math.floor(Date.now() / 1000)
    const baseItem = {
      clientId, capturedAt, status: 'processing', purchaseReference: null,
      message: 'Preparing scan...', updatedAt: capturedAt, attempts: 0,
    }
    upsertLocal(baseItem)
    await idbPut(baseItem)
    try {
      const plain = await downscaleToJpeg(file)
      const sha256 = await sha256Hex(plain)
      const cipher = await encryptForDesktop(plain)
      const ciphertextB64 = bytesToBase64(cipher)
      const item = {
        ...baseItem,
        status: 'pending_upload',
        message: 'Encrypted. Waiting to upload.',
        sizeBytes: cipher.length,
        sha256,
        ciphertextB64,
        contentType: 'application/octet-stream',
        updatedAt: Math.floor(Date.now() / 1000),
      }
      await idbPut(item)
      upsertLocal(item)
      // Fire-and-forget flush; errors are surfaced in item state.
      flush().catch(() => {})
      return clientId
    } catch (e) {
      const failed = {
        ...baseItem,
        status: 'failed_local',
        message: e.message || String(e),
        updatedAt: Math.floor(Date.now() / 1000),
      }
      await idbPut(failed)
      upsertLocal(failed)
      throw e
    }
  }

  async function flush() {
    if (syncing.value) return
    if (!getRelayUrl()) return
    syncing.value = true
    lastSyncError.value = ''
    try {
      // Status-index lookup, not getAll(): terminal history is never read here.
      const pending = await idbGetByStatuses(['pending_upload', 'upload_failed'])
      const nowSec = Math.floor(Date.now() / 1000)
      for (const item of pending) {
        // Bound how long a failing upload keeps its (multi-MB) ciphertext.
        // A capped row becomes terminal `failed` and its payload is dropped;
        // pending_upload rows are always left for retry.
        if (item.status === 'upload_failed' &&
            ((item.attempts || 0) >= MAX_UPLOAD_ATTEMPTS || rowAgeMs(item) > UPLOAD_FAILED_TTL_MS)) {
          const stopped = {
            ...item,
            status: 'failed',
            message: 'Upload stopped after repeated failures or timeout',
            updatedAt: nowSec,
          }
          await idbPut(stopped)
          await idbDeleteCiphertext(item.clientId)
          upsertLocal(stopped)
          continue
        }
        const ciphertextB64 = await idbGetCiphertext(item.clientId)
        if (!ciphertextB64) continue
        try {
          const res = await apiFetch('/api/scans', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              clientId: item.clientId,
              capturedAt: item.capturedAt,
              sizeBytes: item.sizeBytes || 0,
              sha256: item.sha256 || '',
              ciphertextB64,
              contentType: item.contentType || 'application/octet-stream',
            }),
          })
          if (!res.ok) throw new Error(await readError(res, 'Upload failed'))
          const data = await res.json()
          const next = {
            ...item,
            status: data.status || 'uploaded',
            remoteId: data.scanId,
            message: 'Uploaded. Waiting for desktop processing.',
            updatedAt: Math.floor(Date.now() / 1000),
            attempts: (item.attempts || 0) + 1,
          }
          // Keep ciphertext locally until we know it's safely uploaded; can be
          // dropped after upload to save space.
          await idbPut(next)
          await idbDeleteCiphertext(item.clientId)
          upsertLocal(next)
        } catch (e) {
          const next = {
            ...item,
            status: 'upload_failed',
            message: e.message || String(e),
            attempts: (item.attempts || 0) + 1,
            updatedAt: Math.floor(Date.now() / 1000),
          }
          // idbPut leaves the existing payload untouched when no ciphertext is
          // supplied, so the retry still has what it needs.
          await idbPut(next)
          upsertLocal(next)
          lastSyncError.value = next.message
        }
      }
      await pullRemoteStatuses()
      await pruneTerminal()
    } finally {
      syncing.value = false
    }
  }

  async function pullRemoteStatuses() {
    try {
      const res = await apiFetch('/api/scans?since=0')
      if (!res.ok) return
      const data = await res.json()
      const remote = data.scans || []
      if (!remote.length) return
      // Metadata-only read: no ciphertext is deserialized during a status pull.
      const all = await idbGetAll()
      const byClient = new Map(all.map(x => [x.clientId, x]))
      for (const r of remote) {
        const local = byClient.get(r.clientId)
        if (!local) continue
        const terminal = ['imported', 'rejected', 'failed'].includes(r.status)
        const next = {
          ...local,
          status: r.status || local.status,
          remoteId: r.id || local.remoteId,
          purchaseReference: r.purchaseReference ?? local.purchaseReference,
          message: r.message ?? local.message,
          updatedAt: Math.floor(Date.now() / 1000),
        }
        // Once terminal and uploaded, ciphertext no longer needed locally.
        await idbPut(next)
        if (terminal) await idbDeleteCiphertext(local.clientId)
        upsertLocal(next)
      }
    } catch {
      // Status pull is best-effort; local queue remains usable offline.
    }
  }

  async function remove(clientId) {
    await idbDelete(clientId)
    items.value = items.value.filter(x => x.clientId !== clientId)
  }

  return { items, syncing, lastSyncError, load, enqueue, flush, pullRemoteStatuses, remove }
})
