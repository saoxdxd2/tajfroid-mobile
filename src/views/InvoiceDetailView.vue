<template>
  <div class="view">
    <div class="topbar">
      <button @click="$router.push('/sales')">← Sales</button>
      <button @click="load">Refresh</button>
    </div>

    <p v-if="store.error" class="error">{{ store.error }}</p>
    <p v-if="store.loading && !inv">Loading...</p>

    <template v-if="inv">
      <div class="card">
        <div class="header-row">
          <h1>{{ inv.invoiceNumber }}</h1>
          <span class="badge" :class="stateClass(inv.state)">{{ inv.state }}</span>
        </div>
        <p class="muted">{{ inv.customerName || 'Walk-in customer' }}</p>
        <p v-if="inv.dueDate" class="muted">Due: {{ formatDate(inv.dueDate) }}</p>
        <p v-if="inv.notes">{{ inv.notes }}</p>
        <div class="totals">
          <div><span>Total</span><strong>{{ inv.totalAmount }} DH</strong></div>
          <div><span>Paid</span><strong>{{ inv.paidAmount }} DH</strong></div>
          <div :class="{ due: Number(inv.balance) > 0 }"><span>Balance</span><strong>{{ inv.balance }} DH</strong></div>
        </div>
      </div>

      <h3>Items</h3>
      <ul class="line-list">
        <li v-for="it in inv.items" :key="it.id" class="line">
          <div class="line-main">
            <strong>{{ it.productName || it.productSku }}</strong>
            <span class="muted">{{ it.productSku }}</span>
          </div>
          <div class="line-meta">
            <span>{{ it.quantity }} × {{ it.unitPrice }} DH (VAT {{ it.vatRate }}%)</span>
            <span>{{ lineTotal(it) }} DH</span>
          </div>
          <div class="line-meta muted">
            <span>Warranty: {{ it.warrantyMonths }} months</span>
            <button v-if="inv.state === 'Draft'" @click="removeItem(it.id)" class="danger small">Remove</button>
          </div>
        </li>
      </ul>

      <div v-if="inv.state === 'Draft'" class="form-card">
        <h3>Add item</h3>
        <label>Product</label>
        <select v-model="itemForm.productSku" @change="fillFromProduct">
          <option value="">-- Select a product --</option>
          <option v-for="p in activeProducts" :key="p.sku" :value="p.sku">
            {{ p.sku }} — {{ p.name }} (stock {{ p.stock }})
          </option>
        </select>
        <label>Quantity</label>
        <input v-model.number="itemForm.quantity" type="number" min="1" />
        <label>Unit price (DH, VAT excl.)</label>
        <input v-model="itemForm.unitPrice" type="number" step="0.01" />
        <label>VAT %</label>
        <input v-model="itemForm.vatRate" type="number" step="0.01" />
        <label>Warranty (months)</label>
        <input v-model.number="itemForm.warrantyMonths" type="number" min="0" />
        <label>Description override (optional)</label>
        <input v-model="itemForm.descriptionOverride" placeholder="Shown on the invoice" />
        <button @click="addItem" :disabled="!itemForm.productSku || itemForm.quantity < 1">Add to invoice</button>
        <p v-if="itemMsg" class="error">{{ itemMsg }}</p>
      </div>

      <div class="actions">
        <button v-if="inv.state === 'Draft'" @click="act('validate')" class="primary">Validate invoice</button>
        <button v-if="inv.state === 'Validated'" @click="act('issue')">Mark as issued</button>
        <button v-if="canPay" @click="showPay = !showPay">{{ showPay ? 'Cancel payment' : 'Record payment' }}</button>
        <button v-if="inv.state !== 'Archived'" @click="archive" class="danger">Archive</button>
      </div>
      <p v-if="actionMsg" class="error">{{ actionMsg }}</p>

      <div v-if="showPay" class="form-card">
        <h3>Record payment</h3>
        <label>Method</label>
        <select v-model="payForm.method">
          <option value="Cash">Cash</option>
          <option value="Check">Check</option>
          <option value="Bank Transfer">Bank Transfer</option>
        </select>
        <label>Amount (DH)</label>
        <input v-model="payForm.amount" type="number" step="0.01" />
        <label v-if="payForm.method === 'Check'">Registered incoming check</label>
        <select v-if="payForm.method === 'Check'" v-model="payForm.checkId" @change="pickCheck">
          <option value="">-- Select a check --</option>
          <option v-for="c in incomingChecks" :key="c.id" :value="c.id">
            #{{ c.checkNumber }} — {{ c.partyName }} — {{ c.amount }} DH ({{ c.status }})
          </option>
        </select>
        <p v-if="payForm.method === 'Check' && !incomingChecks.length" class="muted">
          Register the check first in Finance → Checks.
        </p>
        <label>Notes</label>
        <input v-model="payForm.notes" placeholder="Notes" />
        <button @click="registerPayment" class="primary">Save payment</button>
        <p v-if="payMsg" class="error">{{ payMsg }}</p>
      </div>

      <h3>Payments</h3>
      <ul class="line-list">
        <li v-for="p in inv.payments" :key="p.id" class="line">
          <div class="line-main">
            <strong>{{ p.method }}</strong>
            <span>{{ p.amount }} DH</span>
          </div>
          <div class="line-meta muted">
            <span v-if="p.reference">Ref: {{ p.reference }}</span>
            <span v-if="p.notes">{{ p.notes }}</span>
            <span>{{ formatDate(p.createdAt) }}</span>
          </div>
        </li>
      </ul>
      <p v-if="!inv.payments.length" class="muted">No payments recorded.</p>
    </template>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useSalesStore } from '../stores/sales'
import { useInventoryStore } from '../stores/inventory'
import { useFinanceStore } from '../stores/finance'

const route = useRoute()
const store = useSalesStore()
const inventory = useInventoryStore()
const finance = useFinanceStore()
const inv = computed(() => store.current)
const itemMsg = ref('')
const actionMsg = ref('')
const payMsg = ref('')
const showPay = ref(false)

const itemForm = ref({
  productSku: '', quantity: 1, unitPrice: '', vatRate: '20',
  warrantyMonths: 12, descriptionOverride: '',
})
const payForm = ref({ method: 'Cash', amount: '', checkId: '', reference: '', notes: '' })

const activeProducts = computed(() => inventory.products.filter(p => p.active))
const incomingChecks = computed(() =>
  finance.checks.filter(c =>
    c.direction === 'Incoming' &&
    ['Pending', 'Deposited', 'Cleared'].includes(c.status) &&
    (!inv.value || !c.customerId || c.customerId === inv.value.customerId)))
const canPay = computed(() =>
  inv.value && (inv.value.state === 'Validated' || inv.value.state === 'Issued'))

onMounted(load)

async function load() {
  await Promise.all([store.fetchInvoice(route.params.id), inventory.fetchProducts(), finance.fetchChecks()])
  if (inv.value) payForm.value.amount = inv.value.balance
}

function lineTotal(it) {
  const base = Number(it.quantity) * Number(it.unitPrice)
  const total = base * (1 + Number(it.vatRate) / 100)
  return total.toFixed(2)
}

function formatDate(ts) {
  if (!ts) return ''
  const n = Number(ts)
  if (!n) return ''
  return new Date(n * 1000).toLocaleDateString()
}

function stateClass(state) {
  return {
    Draft: 'draft', Validated: 'validated', Issued: 'issued', Paid: 'paid', Archived: 'archived',
  }[state] || ''
}

function fillFromProduct() {
  const p = inventory.products.find(x => x.sku === itemForm.value.productSku)
  if (!p) return
  itemForm.value.unitPrice = p.salePrice
  itemForm.value.vatRate = p.vatRate
}

async function addItem() {
  itemMsg.value = ''
  try {
    await store.addItem(route.params.id, { ...itemForm.value })
    itemForm.value = {
      productSku: '', quantity: 1, unitPrice: '', vatRate: '20',
      warrantyMonths: 12, descriptionOverride: '',
    }
  } catch (e) {
    itemMsg.value = e.message
  }
}

async function removeItem(id) {
  if (!confirm('Remove this line?')) return
  try {
    await store.removeItem(route.params.id, id, invoice.value?.updatedAt)
  } catch (e) {
    actionMsg.value = e.message
  }
}

async function act(name) {
  actionMsg.value = ''
  try {
    await store.action(route.params.id, name)
  } catch (e) {
    actionMsg.value = e.message
  }
}

async function archive() {
  if (!confirm('Archive this invoice?')) return
  try {
    await store.archive(route.params.id, invoice.value?.updatedAt)
    window.location.href = '/sales'
  } catch (e) {
    actionMsg.value = e.message
  }
}

function pickCheck() {
  const check = finance.checks.find(c => c.id === payForm.value.checkId)
  if (!check) return
  payForm.value.amount = check.amount
  payForm.value.reference = check.checkNumber
}

async function registerPayment() {
  payMsg.value = ''
  try {
    await store.pay({ invoiceId: route.params.id, ...payForm.value })
    showPay.value = false
    payForm.value = { method: 'Cash', amount: '', checkId: '', reference: '', notes: '' }
    await finance.fetchChecks()
  } catch (e) {
    payMsg.value = e.message
  }
}
</script>

<style scoped>
.topbar { display: flex; justify-content: space-between; margin-bottom: 12px; }
.card { border: 1px solid #ddd; border-radius: 10px; padding: 14px; margin-bottom: 14px; }
.header-row { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.header-row h1 { font-size: 1.25em; margin: 0; }
.totals { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; margin-top: 12px; }
.totals div { background: #f5f5f5; color: #333; border-radius: 8px; padding: 8px; text-align: center; }
.totals span { display: block; font-size: 0.75em; color: #666; }
.totals strong { font-size: 1em; }
.totals div.due strong { color: #c62828; }
.line-list { list-style: none; padding: 0; }
.line { border: 1px solid #ddd; border-radius: 8px; padding: 10px; margin-bottom: 8px; }
.line-main { display: flex; justify-content: space-between; gap: 8px; }
.line-meta { display: flex; justify-content: space-between; gap: 8px; font-size: 0.85em; margin-top: 4px; }
.form-card { background: #f5f5f5; color: #333; padding: 12px; border-radius: 8px; margin: 12px 0; }
.form-card label { display: block; font-size: 0.85em; margin-top: 8px; }
.form-card input, .form-card select { display: block; width: 100%; margin: 4px 0; padding: 8px; }
.actions { display: flex; gap: 8px; flex-wrap: wrap; margin: 12px 0; }
.actions button { flex: 1; min-width: 130px; }
.primary { background: #1f3a5f; color: #fff; border: none; padding: 10px; border-radius: 6px; }
.danger { color: #c62828; }
.small { padding: 2px 6px; font-size: 0.8em; }
.badge { padding: 2px 8px; border-radius: 12px; font-size: 0.8em; background: #eee; color: #333; }
.badge.draft { background: #eceff1; color: #455a64; }
.badge.validated { background: #fff3e0; color: #ef6c00; }
.badge.issued { background: #e3f2fd; color: #1565c0; }
.badge.paid { background: #e8f5e9; color: #2e7d32; }
.badge.archived { background: #f5f5f5; color: #9e9e9e; }
.muted { color: #888; }
.error { color: #c62828; }
</style>
