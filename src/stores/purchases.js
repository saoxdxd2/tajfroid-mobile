import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch, readError, jsonHeaders } from '../lib/api'
import { useMutationStore } from './mutations'


export const usePurchaseStore = defineStore('purchases', () => {
  const purchases = ref([])
  const current = ref(null)
  const loading = ref(false)
  const error = ref('')

  async function fetchAll() {
    loading.value = true
    error.value = ''
    try {
      const res = await apiFetch('/api/purchases')
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
      const res = await apiFetch(`/api/purchases/manage?id=${encodeURIComponent(id)}`)
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
    const body = { ...payload }
    if (!body.id) body.id = crypto.randomUUID()
    try {
      const res = await apiFetch('/api/purchases', {
        method: 'POST', headers: jsonHeaders(), body: JSON.stringify(body),
      })
      if (!res.ok) throw new Error(await readError(res, 'Failed to create purchase'))
      const data = await res.json()
      await fetchAll()
      return data
    } catch (e) {
      if (!navigator.onLine || e instanceof TypeError) {
        await useMutationStore().enqueue({ method: 'POST', path: '/api/purchases', body, entity: 'purchase', entityId: body.id })
        return { id: body.id, queued: true }
      }
      throw e
    }
  }

  async function update(id, payload) {
    const res = await apiFetch(`/api/purchases/manage?id=${encodeURIComponent(id)}`, {
      method: 'PUT', headers: jsonHeaders(), body: JSON.stringify(payload),
    })
    if (res.status === 409) { const d = await res.json().catch(()=>({})); const e = new Error(d.error||'Conflict'); e.conflict=true; e.current=d; throw e }
    if (!res.ok) throw new Error(await readError(res, 'Failed to update purchase'))
    return res.json()
  }

  async function validate(id) {
    await update(id, { action: 'validate' })
    await fetchOne(id)
    await fetchAll()
  }

  async function remove(id) {
    const res = await apiFetch(`/api/purchases/manage?id=${encodeURIComponent(id)}`, {
      method: 'DELETE',
    })
    if (!res.ok) throw new Error(await readError(res, 'Failed to delete purchase'))
    await fetchAll()
  }

  async function addItem(purchaseId, item) {
    const res = await apiFetch('/api/purchases/items', {
      method: 'POST', headers: jsonHeaders(), body: JSON.stringify({ purchaseId, ...item }),
    })
    if (!res.ok) throw new Error(await readError(res, 'Failed to add item'))
    await fetchOne(purchaseId)
    await fetchAll()
    return res.json()
  }

  async function removeItem(purchaseId, itemId) {
    const res = await apiFetch(`/api/purchases/items?id=${encodeURIComponent(itemId)}`, {
      method: 'DELETE',
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
