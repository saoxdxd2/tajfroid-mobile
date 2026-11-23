import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch, jsonHeaders, readError } from '../lib/api'
import { useMutationStore } from './mutations' 

export const useSalesStore = defineStore('sales', () => {
  const invoices = ref([])
  const current = ref(null)
  const loading = ref(false)
  const error = ref('')

  async function fetchInvoices(includeArchived = false) {
    loading.value = true
    error.value = ''
    try {
      const url = `/api/invoices${includeArchived ? '?archived=1' : ''}`
      const res = await apiFetch(url)
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
      const res = await apiFetch(`/api/invoices/manage?id=${encodeURIComponent(id)}`)
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
    const body = { ...payload }
    if (!body.id) body.id = crypto.randomUUID()
    try {
      const res = await apiFetch('/api/invoices', {
        method: 'POST', headers: jsonHeaders(), body: JSON.stringify(body),
      })
      if (!res.ok) throw new Error(await readError(res, 'Failed to create invoice'))
      const data = await res.json()
      await fetchInvoices()
      return data
    } catch (e) {
      if (!navigator.onLine || e instanceof TypeError) {
        await useMutationStore().enqueue({ method: 'POST', path: '/api/invoices', body, entity: 'invoice', entityId: body.id })
        return { id: body.id, queued: true }
      }
      throw e
    }
  }

  async function updateInvoice(id, payload) {
    const res = await apiFetch(`/api/invoices/manage?id=${encodeURIComponent(id)}`, {
      method: 'PUT', headers: jsonHeaders(), body: JSON.stringify(payload),
    })
    if (res.status === 409) { const d = await res.json().catch(()=>({})); const e = new Error(d.error||'Conflict'); e.conflict=true; e.current=d; throw e }
    if (!res.ok) throw new Error(await readError(res, 'Failed to update invoice'))
    return res.json()
  }

  async function action(id, name) {
    const res = await apiFetch(`/api/invoices/manage?id=${encodeURIComponent(id)}`, {
      method: 'PUT', headers: jsonHeaders(), body: JSON.stringify({ action: name }),
    })
    if (!res.ok) throw new Error(await readError(res, `Failed to ${name} invoice`))
    await fetchInvoice(id)
    await fetchInvoices()
  }

  async function archive(id) {
    const res = await apiFetch(`/api/invoices/manage?id=${encodeURIComponent(id)}`, { method: 'DELETE' })
    if (!res.ok) throw new Error(await readError(res, 'Failed to archive invoice'))
    await fetchInvoices()
  }

  async function addItem(invoiceId, item) {
    const res = await apiFetch('/api/invoices/items', {
      method: 'POST', headers: jsonHeaders(), body: JSON.stringify({ invoiceId, ...item }),
    })
    if (!res.ok) throw new Error(await readError(res, 'Failed to add item'))
    await fetchInvoice(invoiceId)
    await fetchInvoices()
    return res.json()
  }

  async function removeItem(invoiceId, itemId) {
    const res = await apiFetch(`/api/invoices/items?id=${encodeURIComponent(itemId)}`, { method: 'DELETE' })
    if (!res.ok) throw new Error(await readError(res, 'Failed to remove item'))
    await fetchInvoice(invoiceId)
    await fetchInvoices()
  }

  async function pay(payload) {
    const body = { ...payload }
    if (!body.id) body.id = crypto.randomUUID() // idempotency key for offline replay/retry
    try {
      const res = await apiFetch('/api/payments', {
        method: 'POST', headers: jsonHeaders(), body: JSON.stringify(body),
      })
      if (!res.ok) throw new Error(await readError(res, 'Failed to register payment'))
      await fetchInvoice(payload.invoiceId)
      await fetchInvoices()
      return res.json()
    } catch (e) {
      if (!navigator.onLine || e instanceof TypeError) {
        await useMutationStore().enqueue({ method: 'POST', path: '/api/payments', body, entity: 'payment', entityId: body.id })
        return { id: body.id, queued: true }
      }
      throw e
    }
  }

  return {
    invoices, current, loading, error,
    fetchInvoices, fetchInvoice, createInvoice, updateInvoice,
    action, archive, addItem, removeItem, pay,
  }
})
