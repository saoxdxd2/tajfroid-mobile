<template>
  <div class="view">
    <h1>Finance</h1>
    <div class="tabs">
      <button v-for="t in tabs" :key="t" :class="{ active: tab === t }" @click="tab = t">{{ t }}</button>
    </div>

    <p v-if="store.error" class="error">{{ store.error }}</p>

    <!-- Overview -->
    <div v-if="tab === 'Overview'">
      <p v-if="store.loading && !summary">Loading...</p>
      <template v-if="summary">
        <div class="cards">
          <div class="card in"><span>Money in</span><strong>{{ summary.inflow }} DH</strong></div>
          <div class="card out"><span>Money out</span><strong>{{ summary.outflow }} DH</strong></div>
          <div class="card net"><span>Net</span><strong>{{ summary.net }} DH</strong></div>
        </div>
        <h3>Debtors — {{ summary.totalOutstanding }} DH</h3>
        <ul class="list">
          <li v-for="d in summary.debtors" :key="d.customerId" class="row">
            <strong>{{ d.companyName }}</strong>
            <span class="due">{{ d.outstanding }} DH</span>
          </li>
        </ul>
        <p v-if="!summary.debtors.length" class="muted">Nobody owes money. 🎉</p>

        <h3>Checks due soon</h3>
        <ul class="list">
          <li v-for="c in summary.upcomingChecks" :key="c.id" class="row">
            <span>{{ c.direction === 'Incoming' ? '📥' : '📤' }} #{{ c.checkNumber }} — {{ c.partyName }}</span>
            <span>{{ c.amount }} DH · {{ formatDate(c.dueDate) }}</span>
          </li>
        </ul>
        <p v-if="!summary.upcomingChecks.length" class="muted">No checks due this week.</p>
      </template>
    </div>

    <!-- Checks -->
    <div v-if="tab === 'Checks'">
      <div class="toolbar">
        <button @click="showCheck = !showCheck">{{ showCheck ? 'Cancel' : '+ New check' }}</button>
        <button @click="refreshChecks">Refresh</button>
      </div>
      <div v-if="showCheck" class="form-card">
        <h3>New check</h3>
        <label>Direction</label>
        <select v-model="checkForm.direction">
          <option value="Incoming">Incoming (from customer)</option>
          <option value="Outgoing">Outgoing (to supplier)</option>
        </select>
        <label>Customer / supplier</label>
        <select v-if="checkForm.direction === 'Incoming'" v-model="checkForm.customerId">
          <option value="">-- Select customer --</option>
          <option v-for="c in customers.customers" :key="c.id" :value="c.id">{{ c.companyName }}</option>
        </select>
        <select v-else v-model="checkForm.supplierId">
          <option value="">-- Select supplier --</option>
          <option v-for="s in suppliers.suppliers" :key="s.id" :value="s.id">{{ s.companyName }}</option>
        </select>
        <label>Check number</label>
        <input v-model="checkForm.checkNumber" placeholder="123456" />
        <label>Amount (DH)</label>
        <input v-model="checkForm.amount" type="number" step="0.01" />
        <label>Due date</label>
        <input v-model="checkForm.dueDate" type="date" />
        <label>Bank</label>
        <input v-model="checkForm.bank" placeholder="Bank" />
        <label>Notes</label>
        <input v-model="checkForm.notes" placeholder="Notes" />
        <button @click="addCheck">Save check</button>
        <p v-if="msg" class="error">{{ msg }}</p>
      </div>
      <ul class="list">
        <li v-for="c in store.checks" :key="c.id" class="block">
          <div class="row">
            <strong>{{ c.direction === 'Incoming' ? '📥' : '📤' }} #{{ c.checkNumber }} — {{ c.partyName }}</strong>
            <span class="badge" :class="c.status.toLowerCase()">{{ c.status }}</span>
          </div>
          <div class="row muted">
            <span>{{ c.amount }} DH · due {{ formatDate(c.dueDate) }}</span>
            <span v-if="c.bank">{{ c.bank }}</span>
          </div>
          <div class="row actions">
            <button v-if="c.status === 'Pending'" @click="move(c, 'Deposited')">Deposit</button>
            <button v-if="c.status === 'Pending' || c.status === 'Deposited'" @click="move(c, 'Cleared')">Cleared</button>
            <button v-if="c.status === 'Deposited'" @click="move(c, 'Bounced')" class="danger">Bounced</button>
            <button v-if="c.status === 'Pending' || c.status === 'Bounced'" @click="move(c, 'Cancelled')" class="danger">Cancel</button>
          </div>
        </li>
      </ul>
      <p v-if="!store.checks.length" class="muted">No checks registered.</p>
    </div>

    <!-- Expenses -->
    <div v-if="tab === 'Expenses'">
      <div class="toolbar">
        <button @click="showExpense = !showExpense">{{ showExpense ? 'Cancel' : '+ New expense' }}</button>
        <button @click="store.fetchExpenses()">Refresh</button>
      </div>
      <div v-if="showExpense" class="form-card">
        <h3>New expense</h3>
        <label>Category</label>
        <select v-model="expenseForm.category">
          <option v-for="c in categories" :key="c" :value="c">{{ c }}</option>
        </select>
        <label>Description</label>
        <input v-model="expenseForm.description" placeholder="What was paid" />
        <label>Amount (DH)</label>
        <input v-model="expenseForm.amount" type="number" step="0.01" />
        <label>Reference (blank = auto)</label>
        <input v-model="expenseForm.reference" placeholder="EXP-26-01" />
        <button @click="addExpense">Save expense</button>
        <p v-if="msg" class="error">{{ msg }}</p>
      </div>
      <ul class="list">
        <li v-for="e in store.expenses" :key="e.id" class="row">
          <div>
            <strong>{{ e.description }}</strong>
            <div class="muted">{{ e.reference }} · {{ e.category }} · {{ formatDate(e.createdAt) }}</div>
          </div>
          <div class="right">
            <strong>{{ e.amount }} DH</strong>
            <button @click="removeExpense(e.id)" class="danger small">Archive</button>
          </div>
        </li>
      </ul>
      <p v-if="!store.expenses.length" class="muted">No expenses recorded.</p>
    </div>

    <!-- Journal -->
    <div v-if="tab === 'Journal'">
      <div class="toolbar"><button @click="store.fetchJournal()">Refresh</button></div>
      <ul class="list">
        <li v-for="e in store.journal" :key="e.id" class="row">
          <div>
            <strong>{{ e.transactionType }}</strong>
            <div class="muted">{{ e.description }} · {{ e.referenceId }}</div>
          </div>
          <div class="right">
            <span :class="Number(e.amount) >= 0 ? 'in-text' : 'out-text'">{{ e.amount }} DH</span>
            <div class="muted">{{ formatDate(e.createdAt) }}</div>
          </div>
        </li>
      </ul>
      <p v-if="!store.journal.length" class="muted">Journal is empty.</p>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useFinanceStore } from '../stores/finance'
import { useCustomerStore } from '../stores/customers'
import { useSupplierStore } from '../stores/suppliers'

const store = useFinanceStore()
const customers = useCustomerStore()
const suppliers = useSupplierStore()
const tabs = ['Overview', 'Checks', 'Expenses', 'Journal']
const tab = ref('Overview')
const summary = computed(() => store.summary)
const msg = ref('')
const showCheck = ref(false)
const showExpense = ref(false)
const categories = ['Salary', 'Utilities', 'Rent', 'Equipment', 'Marketing', 'Travel', 'Tax', 'Other']

const checkForm = ref({
  direction: 'Incoming', customerId: '', supplierId: '', checkNumber: '',
  amount: '', dueDate: '', bank: '', notes: '',
})
const expenseForm = ref({ category: 'Other', description: '', amount: '', reference: '' })

onMounted(async () => {
  await Promise.all([
    store.fetchSummary(), store.fetchChecks(), store.fetchExpenses(),
    store.fetchJournal(), customers.fetchAll(), suppliers.fetchAll(),
  ])
})

watch(tab, (t) => {
  if (t === 'Overview') store.fetchSummary()
  if (t === 'Checks') store.fetchChecks()
  if (t === 'Expenses') store.fetchExpenses()
  if (t === 'Journal') store.fetchJournal()
})

function formatDate(ts) {
  const n = Number(ts)
  return n ? new Date(n * 1000).toLocaleDateString() : ''
}

async function refreshChecks() { await store.fetchChecks() }

async function addCheck() {
  msg.value = ''
  try {
    const payload = { ...checkForm.value }
    if (payload.dueDate) {
      payload.dueDate = Math.floor(new Date(payload.dueDate + 'T12:00:00').getTime() / 1000)
    }
    if (payload.direction === 'Incoming') delete payload.supplierId
    else delete payload.customerId
    await store.createCheck(payload)
    showCheck.value = false
    checkForm.value = {
      direction: 'Incoming', customerId: '', supplierId: '', checkNumber: '',
      amount: '', dueDate: '', bank: '', notes: '',
    }
    await store.fetchSummary()
  } catch (e) {
    msg.value = e.message
  }
}

async function move(check, status) {
  if (!confirm(`Move check #${check.checkNumber} to ${status}?`)) return
  try {
    await store.setCheckStatus(check.id, status, check.updatedAt)
  } catch (e) {
    if (e.conflict) alert('Conflict: check changed on another device. Refreshing.')
    else alert(e.message)
    await store.fetchChecks()
  }
}

async function addExpense() {
  msg.value = ''
  try {
    await store.createExpense({ ...expenseForm.value })
    showExpense.value = false
    expenseForm.value = { category: 'Other', description: '', amount: '', reference: '' }
  } catch (e) {
    msg.value = e.message
  }
}

async function removeExpense(id) {
  if (!confirm('Archive this expense?')) return
  try { await store.archiveExpense(id) } catch (e) { alert(e.message) }
}
</script>

<style scoped>
.tabs { display: flex; gap: 6px; margin-bottom: 12px; overflow-x: auto; }
.tabs button { flex: 1; padding: 8px 10px; white-space: nowrap; border-radius: 8px; }
.tabs button.active { background: #1f3a5f; color: #fff; border-color: #1f3a5f; }
.cards { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; margin-bottom: 14px; }
.card { border-radius: 10px; padding: 10px; text-align: center; background: #f5f5f5; color: #333; }
.card span { display: block; font-size: 0.75em; color: #666; }
.card.in strong { color: #2e7d32; }
.card.out strong { color: #c62828; }
.card.net { background: #1f3a5f; color: #fff; }
.card.net span { color: #cfd8dc; }
.toolbar { display: flex; gap: 8px; margin-bottom: 10px; flex-wrap: wrap; }
.list { list-style: none; padding: 0; }
.row { display: flex; justify-content: space-between; align-items: center; gap: 8px; border-bottom: 1px solid #eee; padding: 8px 0; }
.block { border: 1px solid #ddd; border-radius: 8px; padding: 10px; margin-bottom: 8px; }
.actions { display: flex; gap: 6px; flex-wrap: wrap; margin-top: 6px; }
.actions button { font-size: 0.8em; }
.badge { padding: 2px 8px; border-radius: 12px; font-size: 0.75em; background: #eee; color: #333; }
.badge.pending { background: #fff3e0; color: #ef6c00; }
.badge.deposited { background: #e3f2fd; color: #1565c0; }
.badge.cleared { background: #e8f5e9; color: #2e7d32; }
.badge.bounced { background: #ffebee; color: #c62828; }
.badge.cancelled { background: #f5f5f5; color: #9e9e9e; }
.form-card { background: #f5f5f5; color: #333; padding: 12px; border-radius: 8px; margin-bottom: 12px; }
.form-card label { display: block; font-size: 0.85em; margin-top: 8px; }
.form-card input, .form-card select { display: block; width: 100%; margin: 4px 0; padding: 8px; }
.right { text-align: right; }
.in-text { color: #2e7d32; font-weight: bold; }
.out-text { color: #c62828; font-weight: bold; }
.due { color: #c62828; font-weight: bold; }
.small { padding: 2px 6px; font-size: 0.8em; }
.danger { color: #c62828; }
.error { color: #c62828; }
.muted { color: #888; font-size: 0.9em; }
</style>
