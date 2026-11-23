import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch, readError, jsonHeaders } from '../lib/api'
import { useMutationStore } from './mutations'


export const useFinanceStore = defineStore('finance', () => {
  const summary = ref(null)
  const checks = ref([])
  const expenses = ref([])
  const journal = ref([])
  const loading = ref(false)
  const error = ref('')

  async function fetchSummary(start, end) {
    loading.value = true
    error.value = ''
    try {
      const params = new URLSearchParams()
      if (start) params.set('start', start)
      if (end) params.set('end', end)
      const res = await apiFetch(`/api/finance?${params}`)
      if (!res.ok) throw new Error(await readError(res, 'Failed to load finance summary'))
      summary.value = await res.json()
    } catch (e) {
      error.value = e.message
    } finally {
      loading.value = false
    }
  }

  async function fetchChecks(status = '') {
    try {
      const qs = status ? `?status=${encodeURIComponent(status)}` : ''
      const res = await apiFetch(`/api/checks${qs}`)
      if (!res.ok) throw new Error(await readError(res, 'Failed to load checks'))
      const data = await res.json()
      checks.value = data.checks || []
    } catch (e) {
      error.value = e.message
    }
  }

  async function createCheck(payload) {
    const body = { ...payload }
    if (!body.id) body.id = crypto.randomUUID()
    try {
      const res = await apiFetch('/api/checks', {
        method: 'POST', headers: jsonHeaders(), body: JSON.stringify(body),
      })
      if (!res.ok) throw new Error(await readError(res, 'Failed to create check'))
      await fetchChecks()
      return res.json()
    } catch (e) {
      if (!navigator.onLine || e instanceof TypeError) {
        await useMutationStore().enqueue({ method: 'POST', path: '/api/checks', body, entity: 'check', entityId: body.id })
        return { id: body.id, queued: true }
      }
      throw e
    }
  }

  async function setCheckStatus(id, status, baseUpdatedAt) {
    const payload = { status }
    if (baseUpdatedAt) payload.baseUpdatedAt = baseUpdatedAt
    try {
      const res = await apiFetch(`/api/checks/manage?id=${encodeURIComponent(id)}`, {
        method: 'PUT', headers: jsonHeaders(), body: JSON.stringify(payload),
      })
      if (res.status === 409) { const d = await res.json().catch(()=>({})); const e = new Error(d.error||'Conflict'); e.conflict=true; e.current=d; throw e }
      if (!res.ok) throw new Error(await readError(res, 'Failed to update check'))
    } catch (e) {
      if (!e.conflict && (!navigator.onLine || e instanceof TypeError)) {
        await useMutationStore().enqueue({ method: 'PUT', path: `/api/checks/manage?id=${encodeURIComponent(id)}`, body: payload, entity: 'check', entityId: id, baseUpdatedAt })
        return
      }
      throw e
    }
    await fetchChecks()
    await fetchSummary()
  }

  async function fetchExpenses(includeArchived = false) {
    try {
      const res = await apiFetch(`/api/expenses${includeArchived ? '?archived=1' : ''}`)
      if (!res.ok) throw new Error(await readError(res, 'Failed to load expenses'))
      const data = await res.json()
      expenses.value = data.expenses || []
    } catch (e) {
      error.value = e.message
    }
  }

  async function createExpense(payload) {
    const body = { ...payload }
    if (!body.id) body.id = crypto.randomUUID()
    try {
      const res = await apiFetch('/api/expenses', {
        method: 'POST', headers: jsonHeaders(), body: JSON.stringify(body),
      })
      if (!res.ok) throw new Error(await readError(res, 'Failed to create expense'))
      await fetchExpenses()
      await fetchSummary()
      return res.json()
    } catch (e) {
      if (!navigator.onLine || e instanceof TypeError) {
        await useMutationStore().enqueue({ method: 'POST', path: '/api/expenses', body, entity: 'expense', entityId: body.id })
        return { id: body.id, queued: true }
      }
      throw e
    }
  }

  async function archiveExpense(id, baseUpdatedAt) {
    const q = baseUpdatedAt ? `&baseUpdatedAt=${encodeURIComponent(baseUpdatedAt)}` : ''
    try {
      const res = await apiFetch(`/api/expenses/manage?id=${encodeURIComponent(id)}${q}`, { method: 'DELETE' })
      if (res.status === 409) { const d = await res.json().catch(()=>({})); const e = new Error(d.error||'Conflict'); e.conflict=true; e.current=d; throw e }
      if (!res.ok) throw new Error(await readError(res, 'Failed to archive expense'))
    } catch (e) {
      if (!e.conflict && (!navigator.onLine || e instanceof TypeError)) {
        await useMutationStore().enqueue({ method: 'DELETE', path: `/api/expenses/manage?id=${encodeURIComponent(id)}${q}`, body: null, entity: 'expense', entityId: id, baseUpdatedAt })
        return
      }
      throw e
    }
    await fetchExpenses()
  }

  async function fetchJournal() {
    try {
      const res = await apiFetch('/api/journal')
      if (!res.ok) throw new Error(await readError(res, 'Failed to load journal'))
      const data = await res.json()
      journal.value = data.entries || []
    } catch (e) {
      error.value = e.message
    }
  }

  return {
    summary, checks, expenses, journal, loading, error,
    fetchSummary, fetchChecks, createCheck, setCheckStatus,
    fetchExpenses, createExpense, archiveExpense, fetchJournal,
  }
})
