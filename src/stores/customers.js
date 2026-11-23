import { defineStore } from 'pinia'
import { ref } from 'vue'

function base() { return localStorage.getItem('relayUrl') || '' }
function authHeaders() {
  const token = localStorage.getItem('deviceToken') || ''
  return token ? { Authorization: `Bearer ${token}` } : {}
}

export const useCustomerStore = defineStore('customers', () => {
  const customers = ref([])
  const loading = ref(false)
  const error = ref('')

  async function fetchAll() {
    loading.value = true
    error.value = ''
    try {
      const res = await fetch(`${base()}/api/customers`, { headers: authHeaders() })
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
    const res = await fetch(`${base()}/api/customers`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...authHeaders() },
      body: JSON.stringify(customer),
    })
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error || 'Failed to create customer')
    }
    await fetchAll()
  }

  async function update(id, updates) {
    const res = await fetch(`${base()}/api/customers/manage?id=${encodeURIComponent(id)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', ...authHeaders() },
      body: JSON.stringify(updates),
    })
    if (!res.ok) throw new Error('Failed to update customer')
    await fetchAll()
  }

  async function archive(id) {
    const res = await fetch(`${base()}/api/customers/manage?id=${encodeURIComponent(id)}`, {
      method: 'DELETE',
      headers: authHeaders(),
    })
    if (!res.ok) throw new Error('Failed to archive customer')
    await fetchAll()
  }

  return { customers, loading, error, fetchAll, create, update, archive }
})