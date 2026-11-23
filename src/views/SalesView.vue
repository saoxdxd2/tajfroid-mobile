<template>
  <div class="view">
    <h1>Sales</h1>
    <div class="toolbar">
      <input v-model="q" placeholder="Search N° / customer" />
      <button @click="showNew = !showNew">{{ showNew ? 'Cancel' : '+ New invoice' }}</button>
      <button @click="load">Refresh</button>
    </div>

    <div v-if="showNew" class="form-card">
      <h3>New draft invoice</h3>
      <label>Customer</label>
      <select v-model="form.customerId">
        <option value="">Walk-in (Client de passage)</option>
        <option v-for="c in customers.customers" :key="c.id" :value="c.id">{{ c.companyName }}</option>
      </select>
      <label>Channel</label>
      <select v-model="form.channel">
        <option value="mobile_app">Mobile app</option>
        <option value="store">Store</option>
        <option value="website">Website</option>
      </select>
      <label>Due date (optional)</label>
      <input v-model="form.dueDate" type="date" />
      <label>Notes</label>
      <input v-model="form.notes" placeholder="Notes" />
      <button @click="createInvoice">Create draft</button>
      <p v-if="msg" class="msg">{{ msg }}</p>
    </div>

    <div class="filters">
      <button v-for="f in filters" :key="f.value" :class="{ active: stateFilter === f.value }"
        @click="stateFilter = f.value">{{ f.label }}</button>
    </div>

    <p v-if="store.loading">Loading...</p>
    <p v-if="store.error" class="error">{{ store.error }}</p>

    <ul class="item-list">
      <li v-for="inv in filtered" :key="inv.id" class="item" @click="$router.push('/sales/' + inv.id)">
        <div class="item-header">
          <strong>{{ inv.invoiceNumber }}</strong>
          <span class="badge" :class="stateClass(inv.state)">{{ inv.state }}</span>
        </div>
        <div class="item-details">
          <span>{{ inv.customerName || '—' }}</span>
          <span>{{ inv.totalAmount }} DH</span>
          <span v-if="Number(inv.balance) > 0" class="due">Due: {{ inv.balance }} DH</span>
        </div>
      </li>
    </ul>
    <p v-if="!store.loading && !filtered.length" class="muted">No invoices yet.</p>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useSalesStore } from '../stores/sales'
import { useCustomerStore } from '../stores/customers'

const store = useSalesStore()
const customers = useCustomerStore()
const router = useRouter()
const q = ref('')
const showNew = ref(false)
const msg = ref('')
const stateFilter = ref('all')
const form = ref({ customerId: '', channel: 'mobile_app', dueDate: '', notes: '' })

const filters = [
  { label: 'All', value: 'all' },
  { label: 'Draft', value: 'Draft' },
  { label: 'To pay', value: 'Validated' },
  { label: 'Paid', value: 'Paid' },
]

const filtered = computed(() => {
  const term = q.value.toLowerCase().trim()
  return store.invoices.filter(inv => {
    if (stateFilter.value !== 'all' && inv.state !== stateFilter.value) return false
    if (!term) return true
    return (`${inv.invoiceNumber} ${inv.customerName}`).toLowerCase().includes(term)
  })
})

function stateClass(state) {
  return {
    Draft: 'draft', Validated: 'validated', Issued: 'issued', Paid: 'paid', Archived: 'archived',
  }[state] || ''
}

onMounted(async () => {
  await Promise.all([store.fetchInvoices(), customers.fetchAll()])
})

async function load() {
  await Promise.all([store.fetchInvoices(), customers.fetchAll()])
}

async function createInvoice() {
  try {
    const payload = {
      customerId: form.value.customerId,
      channel: form.value.channel,
      notes: form.value.notes,
    }
    if (form.value.dueDate) {
      payload.dueDate = Math.floor(new Date(form.value.dueDate + 'T12:00:00').getTime() / 1000)
    }
    const data = await store.createInvoice(payload)
    showNew.value = false
    router.push('/sales/' + data.id)
  } catch (e) {
    msg.value = 'Error: ' + e.message
  }
}
</script>

<style scoped>
.toolbar { display: flex; gap: 8px; margin-bottom: 12px; flex-wrap: wrap; }
.toolbar input { flex: 1; min-width: 140px; padding: 8px; }
.form-card { background: #f5f5f5; color: #333; padding: 12px; border-radius: 8px; margin-bottom: 12px; }
.form-card label { display: block; font-size: 0.85em; margin-top: 8px; }
.form-card input, .form-card select { display: block; width: 100%; margin: 4px 0; padding: 8px; }
.filters { display: flex; gap: 6px; margin-bottom: 10px; flex-wrap: wrap; }
.filters button.active { background: #1f3a5f; color: #fff; }
.item-list { list-style: none; padding: 0; }
.item { border: 1px solid #ddd; border-radius: 8px; padding: 12px; margin-bottom: 8px; cursor: pointer; }
.item-header { display: flex; justify-content: space-between; align-items: center; }
.item-details { display: flex; gap: 12px; flex-wrap: wrap; font-size: 0.9em; color: #666; margin-top: 4px; }
.due { color: #c62828; font-weight: bold; }
.badge { padding: 2px 8px; border-radius: 12px; font-size: 0.8em; background: #eee; color: #333; }
.badge.draft { background: #eceff1; color: #455a64; }
.badge.validated { background: #fff3e0; color: #ef6c00; }
.badge.issued { background: #e3f2fd; color: #1565c0; }
.badge.paid { background: #e8f5e9; color: #2e7d32; }
.badge.archived { background: #f5f5f5; color: #9e9e9e; }
.msg { color: #c62828; }
.error { color: #c62828; }
.muted { color: #888; }
</style>
