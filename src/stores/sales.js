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
    try {
      const res = await apiFetch(`/api/invoices/manage?id=${encodeURIComponent(id)}`, {
        method: 'PUT', headers: jsonHeaders(), body: JSON.stringify(payload),
      })
      if (res.status === 409) {
        const d = await res.json().catch(()=>({}))
        await useMutationStore().recordConflict({ method:'PUT', path:`/api/invoices/manage?id=${encodeURIComponent(id)}`, body: payload, entity:'invoice', entityId:id, baseUpdatedAt: payload.baseUpdatedAt, conflict:d, error:d.error })
        const e = new Error(d.error||'Conflict'); e.conflict=true; e.current=d; throw e
      }
      if (!res.ok) throw new Error(await readError(res, 'Failed to update invoice'))
      return res.json()
    } catch (e) {
      if (!e.conflict && (!navigator.onLine || e instanceof TypeError)) {
        await useMutationStore().enqueue({ method:'PUT', path:`/api/invoices/manage?id=${encodeURIComponent(id)}`, body: payload, entity:'invoice', entityId:id, baseUpdatedAt: payload.baseUpdatedAt })
        return { queued: true }
      }
      throw e
    }
  }

  async function action(id, name) {
    const body = { action: name }
    if (current.value?.id === id && current.value?.updatedAt) body.baseUpdatedAt = current.value.updatedAt
    try {
      const res = await apiFetch(`/api/invoices/manage?id=${encodeURIComponent(id)}`, {
        method: 'PUT', headers: jsonHeaders(), body: JSON.stringify(body),
      })
      if (res.status === 409) {
        const d = await res.json().catch(()=>({}))
        await useMutationStore().recordConflict({ method:'PUT', path:`/api/invoices/manage?id=${encodeURIComponent(id)}`, body, entity:'invoice', entityId:id, baseUpdatedAt: body.baseUpdatedAt, conflict:d, error:d.error })
        const e = new Error(d.error||'Conflict'); e.conflict=true; e.current=d; throw e
      }
      if (!res.ok) throw new Error(await readError(res, `Failed to ${name} invoice`))
    } catch (e) {
      if (!e.conflict && (!navigator.onLine || e instanceof TypeError)) {
        await useMutationStore().enqueue({ method: 'PUT', path: `/api/invoices/manage?id=${encodeURIComponent(id)}`, body, entity: 'invoice', entityId: id, baseUpdatedAt: body.baseUpdatedAt })
        return
      }
      throw e
    }
    await fetchInvoice(id)
    await fetchInvoices()
  }

  async function archive(id, baseUpdatedAt) {
    const q = baseUpdatedAt ? `&baseUpdatedAt=${encodeURIComponent(baseUpdatedAt)}` : ''
    try {
      const res = await apiFetch(`/api/invoices/manage?id=${encodeURIComponent(id)}${q}`, { method: 'DELETE' })
      if (res.status === 409) {
        const d = await res.json().catch(()=>({}))
        await useMutationStore().recordConflict({ method:'DELETE', path:`/api/invoices/manage?id=${encodeURIComponent(id)}${q}`, body:null, entity:'invoice', entityId:id, baseUpdatedAt, conflict:d, error:d.error })
        const e = new Error(d.error||'Conflict'); e.conflict=true; e.current=d; throw e
      }
      if (!res.ok) throw new Error(await readError(res, 'Failed to archive invoice'))
    } catch (e) {
      if (!e.conflict && (!navigator.onLine || e instanceof TypeError)) {
        await useMutationStore().enqueue({ method: 'DELETE', path: `/api/invoices/manage?id=${encodeURIComponent(id)}${q}`, body: null, entity: 'invoice', entityId: id, baseUpdatedAt })
        return
      }
      throw e
    }
    await fetchInvoices()
  }

  async function addItem(invoiceId, item) {
    const body = { invoiceId, ...item }
    if (!body.id) body.id = crypto.randomUUID()
    try {
      const res = await apiFetch('/api/invoices/items', {
        method: 'POST', headers: jsonHeaders(), body: JSON.stringify(body),
      })
      if (res.status === 409) {
        const d = await res.json().catch(()=>({}))
        await useMutationStore().recordConflict({ method:'POST', path:'/api/invoices/items', body, entity:'invoice_item', entityId: body.id, conflict:d, error:d.error })
        const e = new Error(d.error||'Conflict'); e.conflict=true; e.current=d; throw e
      }
      if (!res.ok) throw new Error(await readError(res, 'Failed to add item'))
      await fetchInvoice(invoiceId)
      await fetchInvoices()
      return res.json()
    } catch (e) {
      if (!navigator.onLine || e instanceof TypeError) {
        await useMutationStore().enqueue({ method: 'POST', path: '/api/invoices/items', body, entity: 'invoice_item', entityId: body.id })
        return { id: body.id, queued: true }
      }
      throw e
    }
  }

  async function removeItem(invoiceId, itemId) {
    const path = `/api/invoices/items?id=${encodeURIComponent(itemId)}`
    try {
      const res = await apiFetch(path, { method: 'DELETE' })
      if (res.status === 409) {
        const d = await res.json().catch(()=>({}))
        await useMutationStore().recordConflict({ method:'DELETE', path, body:null, entity:'invoice_item', entityId: itemId, conflict:d, error:d.error })
        const e = new Error(d.error||'Conflict'); e.conflict=true; e.current=d; throw e
      }
      if (!res.ok) throw new Error(await readError(res, 'Failed to remove item'))
    } catch (e) {
      if (!navigator.onLine || e instanceof TypeError) {
        await useMutationStore().enqueue({ method: 'DELETE', path, body: null, entity: 'invoice_item', entityId: itemId })
        return
      }
      throw e
    }
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
