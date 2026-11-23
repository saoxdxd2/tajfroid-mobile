import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch, jsonHeaders, readError } from '../lib/api'
import { useMutationStore } from './mutations'

export const useSupplierStore = defineStore('suppliers', () => {
  const suppliers = ref([])
  const loading = ref(false)
  const error = ref('')

  async function fetchAll() {
    loading.value = true
    error.value = ''
    try {
      const res = await apiFetch('/api/suppliers')
      if (!res.ok) throw new Error('Failed to fetch suppliers')
      const data = await res.json()
      suppliers.value = data.suppliers || []
    } catch (e) {
      error.value = e.message
    } finally {
      loading.value = false
    }
  }

  async function create(supplier) {
    const body = { ...supplier }
    if (!body.id) body.id = crypto.randomUUID()
    try {
      const res = await apiFetch('/api/suppliers', {
        method: 'POST',
        headers: jsonHeaders(),
        body: JSON.stringify(body),
      })
      if (!res.ok) throw new Error(await readError(res, 'Failed to create supplier'))
    } catch (e) {
      if (!navigator.onLine || e instanceof TypeError) {
        await useMutationStore().enqueue({ method: 'POST', path: '/api/suppliers', body, entity: 'supplier', entityId: body.id })
        suppliers.value.unshift({ ...body, _pending: true })
        return
      }
      throw e
    }
    await fetchAll()
  }

  async function update(id, updates) {
    const body = { ...updates }
    try {
      const res = await apiFetch(`/api/suppliers/manage?id=${encodeURIComponent(id)}`, {
        method: 'PUT',
        headers: jsonHeaders(),
        body: JSON.stringify(body),
      })
      if (res.status === 409) { const d = await res.json().catch(()=>({})); const e = new Error(d.error||'Conflict: record changed'); e.conflict=true; e.current=d; throw e }
      if (!res.ok) throw new Error(await readError(res, 'Failed to update supplier'))
    } catch (e) {
      if (!e.conflict && (!navigator.onLine || e instanceof TypeError)) {
        await useMutationStore().enqueue({ method: 'PUT', path: `/api/suppliers/manage?id=${encodeURIComponent(id)}`, body, entity: 'supplier', entityId: id, baseUpdatedAt: body.baseUpdatedAt })
        return
      }
      throw e
    }
    await fetchAll()
  }

  async function archive(id, baseUpdatedAt) {
    const q = baseUpdatedAt ? `&baseUpdatedAt=${encodeURIComponent(baseUpdatedAt)}` : ''
    try {
      const res = await apiFetch(`/api/suppliers/manage?id=${encodeURIComponent(id)}${q}`, { method: 'DELETE' })
      if (res.status === 409) { const d = await res.json().catch(()=>({})); const e = new Error(d.error||'Conflict'); e.conflict=true; e.current=d; throw e }
      if (!res.ok) throw new Error('Failed to archive supplier')
    } catch (e) {
      if (!e.conflict && (!navigator.onLine || e instanceof TypeError)) {
        await useMutationStore().enqueue({ method: 'DELETE', path: `/api/suppliers/manage?id=${encodeURIComponent(id)}${q}`, body: null, entity: 'supplier', entityId: id, baseUpdatedAt })
        return
      }
      throw e
    }
    await fetchAll()
  }

  return { suppliers, loading, error, fetchAll, create, update, archive }
})