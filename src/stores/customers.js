import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch, jsonHeaders, readError } from '../lib/api'
import { useMutationStore } from './mutations'

export const useCustomerStore = defineStore('customers', () => {
  const customers = ref([])
  const loading = ref(false)
  const error = ref('')

  async function fetchAll() {
    loading.value = true
    error.value = ''
    try {
      const res = await apiFetch('/api/customers')
      if (!res.ok) throw new Error('Failed to fetch customers')
      const data = await res.json()
      customers.value = data.customers || []
    } catch (e) {
      error.value = e.message
    } finally {
      loading.value = false
    }
  }

  async function create(customer) {
    const body = { ...customer }
    if (!body.id) body.id = crypto.randomUUID()
    try {
      const res = await apiFetch('/api/customers', {
        method: 'POST',
        headers: jsonHeaders(),
        body: JSON.stringify(body),
      })
      if (!res.ok) throw new Error(await readError(res, 'Failed to create customer'))
    } catch (e) {
      if (!navigator.onLine || e instanceof TypeError) {
        await useMutationStore().enqueue({ method: 'POST', path: '/api/customers', body, entity: 'customer', entityId: body.id })
        customers.value.unshift({ ...body, _pending: true })
        return
      }
      throw e
    }
    await fetchAll()
  }

  async function update(id, updates) {
    const body = { ...updates }
    try {
      const res = await apiFetch(`/api/customers/manage?id=${encodeURIComponent(id)}`, {
        method: 'PUT',
        headers: jsonHeaders(),
        body: JSON.stringify(body),
      })
      if (res.status === 409) { const d = await res.json().catch(()=>({})); const e = new Error(d.error||'Conflict: record changed'); e.conflict=true; e.current=d; throw e }
      if (!res.ok) throw new Error(await readError(res, 'Failed to update customer'))
    } catch (e) {
      if (!e.conflict && (!navigator.onLine || e instanceof TypeError)) {
        await useMutationStore().enqueue({ method: 'PUT', path: `/api/customers/manage?id=${encodeURIComponent(id)}`, body, entity: 'customer', entityId: id, baseUpdatedAt: body.baseUpdatedAt })
        return
      }
      throw e
    }
    await fetchAll()
  }

  async function archive(id) {
    const res = await apiFetch(`/api/customers/manage?id=${encodeURIComponent(id)}`, { method: 'DELETE' })
    if (!res.ok) throw new Error('Failed to archive customer')
    await fetchAll()
  }

  return { customers, loading, error, fetchAll, create, update, archive }
})