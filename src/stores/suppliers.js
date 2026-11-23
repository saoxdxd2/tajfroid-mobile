import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch, jsonHeaders, readError } from '../lib/api'

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
    const res = await apiFetch('/api/suppliers', {
      method: 'POST',
      headers: jsonHeaders(),
      body: JSON.stringify(supplier),
    })
    if (!res.ok) {
      throw new Error(await readError(res, 'Failed to create supplier'))
    }
    await fetchAll()
  }

  async function update(id, updates) {
    const res = await apiFetch(`/api/suppliers/manage?id=${encodeURIComponent(id)}`, {
      method: 'PUT',
      headers: jsonHeaders(),
      body: JSON.stringify(updates),
    })
    if (res.status === 409) { const d = await res.json().catch(()=>({})); const e = new Error(d.error||'Conflict: record changed'); e.conflict=true; e.current=d; throw e }
    if (!res.ok) throw new Error(await readError(res, 'Failed to update supplier'))
    await fetchAll()
  }

  async function archive(id) {
    const res = await apiFetch(`/api/suppliers/manage?id=${encodeURIComponent(id)}`, { method: 'DELETE' })
    if (!res.ok) throw new Error('Failed to archive supplier')
    await fetchAll()
  }

  return { suppliers, loading, error, fetchAll, create, update, archive }
})