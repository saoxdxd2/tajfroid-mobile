import { defineStore } from 'pinia'
import { ref } from 'vue'

function base() { return localStorage.getItem('relayUrl') || '' }
function authHeaders() {
  const token = localStorage.getItem('deviceToken') || ''
  return token ? { Authorization: `Bearer ${token}` } : {}
}

export const useSupplierStore = defineStore('suppliers', () => {
  const suppliers = ref([])
  const loading = ref(false)
  const error = ref('')

  async function fetchAll() {
    loading.value = true
    error.value = ''
    try {
      const res = await fetch(`${base()}/api/suppliers`, { headers: authHeaders() })
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
    const res = await fetch(`${base()}/api/suppliers`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...authHeaders() },
      body: JSON.stringify(supplier),
    })
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error || 'Failed to create supplier')
    }
    await fetchAll()
  }

  async function update(id, updates) {
    const res = await fetch(`${base()}/api/suppliers/manage?id=${encodeURIComponent(id)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', ...authHeaders() },
      body: JSON.stringify(updates),
    })
    if (!res.ok) throw new Error('Failed to update supplier')
    await fetchAll()
  }

  async function archive(id) {
    const res = await fetch(`${base()}/api/suppliers/manage?id=${encodeURIComponent(id)}`, {
      method: 'DELETE',
      headers: authHeaders(),
    })
    if (!res.ok) throw new Error('Failed to archive supplier')
    await fetchAll()
  }

  return { suppliers, loading, error, fetchAll, create, update, archive }
})