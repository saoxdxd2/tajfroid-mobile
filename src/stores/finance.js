import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch, readError, jsonHeaders } from '../lib/api'


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
    const res = await apiFetch('/api/checks', {
      method: 'POST', headers: jsonHeaders(), body: JSON.stringify(payload),
    })
    if (!res.ok) throw new Error(await readError(res, 'Failed to create check'))
    await fetchChecks()
    return res.json()
  }

  async function setCheckStatus(id, status) {
    const res = await apiFetch(`/api/checks/manage?id=${encodeURIComponent(id)}`, {
      method: 'PUT', headers: jsonHeaders(), body: JSON.stringify({ status }),
    })
    if (!res.ok) throw new Error(await readError(res, 'Failed to update check'))
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
    const res = await apiFetch('/api/expenses', {
      method: 'POST', headers: jsonHeaders(), body: JSON.stringify(payload),
    })
    if (!res.ok) throw new Error(await readError(res, 'Failed to create expense'))
    await fetchExpenses()
    await fetchSummary()
    return res.json()
  }

  async function archiveExpense(id) {
    const res = await apiFetch(`/api/expenses/manage?id=${encodeURIComponent(id)}`, {
      method: 'DELETE', headers: authHeaders(),
    })
    if (!res.ok) throw new Error(await readError(res, 'Failed to archive expense'))
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
