import { defineStore } from 'pinia'
import { ref } from 'vue'

function base() { return localStorage.getItem('relayUrl') || '' }
function authHeaders() {
  const token = localStorage.getItem('deviceToken') || ''
  return token ? { Authorization: `Bearer ${token}` } : {}
}
function jsonHeaders() {
  return { 'Content-Type': 'application/json', ...authHeaders() }
}
async function readError(res, fallback) {
  const text = await res.text().catch(() => '')
  return text || fallback
}

export const usePurchaseStore = defineStore('purchases', () => {
  const purchases = ref([])
  const current = ref(null)
  const loading = ref(false)
  const error = ref('')

  async function fetchAll() {
    loading.value = true
    error.value = ''
    try {
      const res = await fetch(`${base()}/api/purchases`, { headers: authHeaders() })
      if (!res.ok) throw new Error(await readError(res, 'Failed to fetch purchases'))
      const data = await res.json()
      purchases.value = data.purchases || []
    } catch (e) {
      error.value = e.message
    } finally {
      loading.value = false
    }
  }

  async function fetchOne(id) {
    loading.value = true
    error.value = ''
    try {
      const res = await fetch(`${base()}/api/purchases/manage?id=${encodeURIComponent(id)}`, { headers: authHeaders() })
      if (!res.ok) throw new Error(await readError(res, 'Failed to load purchase'))
      current.value = await res.json()
      return current.value
    } catch (e) {
      error.value = e.message
      throw e
    } finally {
      loading.value = false
    }
  }

  async function create(payload) {
    const res = await fetch(`${base()}/api/purchases`, {
      method: 'POST', headers: jsonHeaders(), body: JSON.stringify(payload),
    })
    if (!res.ok) throw new Error(await readError(res, 'Failed to create purchase'))
    const data = await res.json()
    await fetchAll()
    return data
  }

  async function update(id, payload) {
    const res = await fetch(`${base()}/api/purchases/manage?id=${encodeURIComponent(id)}`, {
      method: 'PUT', headers: jsonHeaders(), body: JSON.stringify(payload),
    })
    if (!res.ok) throw new Error(await readError(res, 'Failed to update purchase'))
    return res.json()
  }

  async function validate(id) {
    await update(id, { action: 'validate' })
    await fetchOne(id)
    await fetchAll()
  }

  async function remove(id) {
    const res = await fetch(`${base()}/api/purchases/manage?id=${encodeURIComponent(id)}`, {
      method: 'DELETE', headers: authHeaders(),
    })
    if (!res.ok) throw new Error(await readError(res, 'Failed to delete purchase'))
    await fetchAll()
  }

  async function addItem(purchaseId, item) {
    const res = await fetch(`${base()}/api/purchases/items`, {
      method: 'POST', headers: jsonHeaders(), body: JSON.stringify({ purchaseId, ...item }),
    })
    if (!res.ok) throw new Error(await readError(res, 'Failed to add item'))
    await fetchOne(purchaseId)
    await fetchAll()
    return res.json()
  }

  async function removeItem(purchaseId, itemId) {
    const res = await fetch(`${base()}/api/purchases/items?id=${encodeURIComponent(itemId)}`, {
      method: 'DELETE', headers: authHeaders(),
    })
    if (!res.ok) throw new Error(await readError(res, 'Failed to remove item'))
    await fetchOne(purchaseId)
    await fetchAll()
  }

  return {
    purchases, current, loading, error,
    fetchAll, fetchOne, create, update, validate, remove, addItem, removeItem,
  }
})
