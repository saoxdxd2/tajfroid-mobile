import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch, jsonHeaders, readError } from '../lib/api'

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
    const res = await apiFetch('/api/customers', {
      method: 'POST',
      headers: jsonHeaders(),
      body: JSON.stringify(customer),
    })
    if (!res.ok) {
      throw new Error(await readError(res, 'Failed to create customer'))
    }
    await fetchAll()
  }

  async function update(id, updates) {
    const res = await apiFetch(`/api/customers/manage?id=${encodeURIComponent(id)}`, {
      method: 'PUT',
      headers: jsonHeaders(),
      body: JSON.stringify(updates),
    })
    if (!res.ok) throw new Error('Failed to update customer')
    await fetchAll()
  }

  async function archive(id) {
    const res = await apiFetch(`/api/customers/manage?id=${encodeURIComponent(id)}`, { method: 'DELETE' })
    if (!res.ok) throw new Error('Failed to archive customer')
    await fetchAll()
  }

  return { customers, loading, error, fetchAll, create, update, archive }
})