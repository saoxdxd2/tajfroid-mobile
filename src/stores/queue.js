import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch, readError, getRelayUrl } from '../lib/api'

const DB_NAME = 'tajfroid-scan-queue'
const DB_VERSION = 1
const STORE = 'scans'
const MAX_PX = 2000
const MAX_BYTES = 2_500_000
const JPEG_QUALITY = 0.82

function openDb() {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION)
    req.onupgradeneeded = () => {
      const db = req.result
      if (!db.objectStoreNames.contains(STORE)) {
        const os = db.createObjectStore(STORE, { keyPath: 'clientId' })
        os.createIndex('status', 'status', { unique: false })
        os.createIndex('updatedAt', 'updatedAt', { unique: false })
      }
    }
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error || new Error('IndexedDB open failed'))
  })
}

async function idbGetAll() {
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

async function idbDelete(clientId) {
  const db = await openDb()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE, 'readwrite')
    tx.objectStore(STORE).delete(clientId)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
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
    items.value = all.map(x => ({ ...x, ciphertextB64: undefined }))
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
      const all = await idbGetAll()
      const pending = all.filter(x => ['pending_upload', 'upload_failed'].includes(x.status) && x.ciphertextB64)
      for (const item of pending) {
        try {
          const res = await apiFetch('/api/scans', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              clientId: item.clientId,
              capturedAt: item.capturedAt,
              sizeBytes: item.sizeBytes || 0,
              sha256: item.sha256 || '',
              ciphertextB64: item.ciphertextB64,
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
          delete next.ciphertextB64
          await idbPut(next)
          upsertLocal(next)
        } catch (e) {
          const next = {
            ...item,
            status: 'upload_failed',
            message: e.message || String(e),
            attempts: (item.attempts || 0) + 1,
            updatedAt: Math.floor(Date.now() / 1000),
          }
          await idbPut(next)
          upsertLocal(next)
          lastSyncError.value = next.message
        }
      }
      await pullRemoteStatuses()
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
        if (terminal) delete next.ciphertextB64
        await idbPut(next)
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
