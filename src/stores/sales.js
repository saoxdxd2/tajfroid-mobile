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

export const useSalesStore = defineStore('sales', () => {
  const invoices = ref([])
  const current = ref(null)
  const loading = ref(false)
  const error = ref('')

  async function fetchInvoices(includeArchived = false) {
    loading.value = true
    error.value = ''
    try {
      const url = `${base()}/api/invoices${includeArchived ? '?archived=1' : ''}`
      const res = await fetch(url, { headers: authHeaders() })
      if (!res.ok) throw new Error(await readError(res, 'Failed to fetch invoices'))
      const data = await res.json()
      invoices.value = data.invoices || []
    } catch (e) {
      error.value = e.message
    } finally {
      loading.value = false
    }
  }

  async function fetchInvoice(id) {
    loading.value = true
    error.value = ''
    try {
      const res = await fetch(`${base()}/api/invoices/manage?id=${encodeURIComponent(id)}`, { headers: authHeaders() })
      if (!res.ok) throw new Error(await readError(res, 'Failed to load invoice'))
      current.value = await res.json()
      return current.value
    } catch (e) {
      error.value = e.message
      throw e
    } finally {
      loading.value = false
    }
  }

  async function createInvoice(payload) {
    const res = await fetch(`${base()}/api/invoices`, {
      method: 'POST', headers: jsonHeaders(), body: JSON.stringify(payload),
    })
    if (!res.ok) throw new Error(await readError(res, 'Failed to create invoice'))
    const data = await res.json()
    await fetchInvoices()
    return data
  }

  async function updateInvoice(id, payload) {
    const res = await fetch(`${base()}/api/invoices/manage?id=${encodeURIComponent(id)}`, {
      method: 'PUT', headers: jsonHeaders(), body: JSON.stringify(payload),
    })
    if (!res.ok) throw new Error(await readError(res, 'Failed to update invoice'))
    return res.json()
  }

  async function action(id, name) {
    const res = await fetch(`${base()}/api/invoices/manage?id=${encodeURIComponent(id)}`, {
      method: 'PUT', headers: jsonHeaders(), body: JSON.stringify({ action: name }),
    })
    if (!res.ok) throw new Error(await readError(res, `Failed to ${name} invoice`))
    await fetchInvoice(id)
    await fetchInvoices()
  }

  async function archive(id) {
    const res = await fetch(`${base()}/api/invoices/manage?id=${encodeURIComponent(id)}`, {
      method: 'DELETE', headers: authHeaders(),
    })
    if (!res.ok) throw new Error(await readError(res, 'Failed to archive invoice'))
    await fetchInvoices()
  }

  async function addItem(invoiceId, item) {
    const res = await fetch(`${base()}/api/invoices/items`, {
      method: 'POST', headers: jsonHeaders(), body: JSON.stringify({ invoiceId, ...item }),
    })
    if (!res.ok) throw new Error(await readError(res, 'Failed to add item'))
    await fetchInvoice(invoiceId)
    await fetchInvoices()
    return res.json()
  }

  async function removeItem(invoiceId, itemId) {
    const res = await fetch(`${base()}/api/invoices/items?id=${encodeURIComponent(itemId)}`, {
      method: 'DELETE', headers: authHeaders(),
    })
    if (!res.ok) throw new Error(await readError(res, 'Failed to remove item'))
    await fetchInvoice(invoiceId)
    await fetchInvoices()
  }

  async function pay(payload) {
    const res = await fetch(`${base()}/api/payments`, {
      method: 'POST', headers: jsonHeaders(), body: JSON.stringify(payload),
    })
    if (!res.ok) throw new Error(await readError(res, 'Failed to register payment'))
    await fetchInvoice(payload.invoiceId)
    await fetchInvoices()
    return res.json()
  }

  return {
    invoices, current, loading, error,
    fetchInvoices, fetchInvoice, createInvoice, updateInvoice,
    action, archive, addItem, removeItem, pay,
  }
})
