<template>
  <div class="view">
    <h1>Purchases</h1>
    <div class="toolbar">
      <input v-model="q" placeholder="Search reference / supplier" />
      <button @click="showNew = !showNew">{{ showNew ? 'Cancel' : '+ New purchase' }}</button>
      <button @click="load">Refresh</button>
    </div>

    <div v-if="showNew" class="form-card">
      <h3>New draft purchase</h3>
      <label>Supplier</label>
      <select v-model="form.supplierId">
        <option value="">-- Select supplier --</option>
        <option v-for="s in suppliers.suppliers" :key="s.id" :value="s.id">{{ s.companyName }}</option>
      </select>
      <label>Reference (blank = auto PUR-YY-NN)</label>
      <input v-model="form.reference" placeholder="PUR-26-01" />
      <label>Supplier invoice N°</label>
      <input v-model="form.supplierInvoiceNo" placeholder="Invoice number" />
      <label>Purchase date</label>
      <input v-model="form.purchaseDate" type="date" />
      <button @click="createPurchase" :disabled="!form.supplierId">Create draft</button>
      <p v-if="msg" class="error">{{ msg }}</p>
    </div>

    <div class="filters">
      <button v-for="f in filters" :key="f.value" :class="{ active: stateFilter === f.value }"
        @click="stateFilter = f.value">{{ f.label }}</button>
    </div>

    <p v-if="store.loading">Loading...</p>
    <p v-if="store.error" class="error">{{ store.error }}</p>

    <ul class="item-list">
      <li v-for="p in filtered" :key="p.id" class="item" @click="$router.push('/purchases/' + p.id)">
        <div class="item-header">
          <strong>{{ p.reference }}</strong>
          <span class="badge" :class="p.state === 'Validated' ? 'validated' : 'draft'">{{ p.state }}</span>
        </div>
        <div class="item-details">
          <span>{{ p.supplierName || '—' }}</span>
          <span>{{ p.totalAmount }} DH</span>
          <span v-if="p.supplierInvoiceNo">Inv: {{ p.supplierInvoiceNo }}</span>
        </div>
      </li>
    </ul>
    <p v-if="!store.loading && !filtered.length" class="empty-state">{{ COPY.empty.purchases }}</p>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { usePurchaseStore } from '../stores/purchases'
import { useSupplierStore } from '../stores/suppliers'
import { COPY } from '../lib/copy'

const store = usePurchaseStore()
const suppliers = useSupplierStore()
const router = useRouter()
const q = ref('')
const showNew = ref(false)
const msg = ref('')
const stateFilter = ref('all')
const form = ref({ supplierId: '', reference: '', supplierInvoiceNo: '', purchaseDate: '' })

const filters = [
  { label: 'All', value: 'all' },
  { label: 'Draft', value: 'Draft' },
  { label: 'Validated', value: 'Validated' },
]

const filtered = computed(() => {
  const term = q.value.toLowerCase().trim()
  return store.purchases.filter(p => {
    if (stateFilter.value !== 'all' && p.state !== stateFilter.value) return false
    if (!term) return true
    return (`${p.reference} ${p.supplierName} ${p.supplierInvoiceNo}`).toLowerCase().includes(term)
  })
})

onMounted(async () => {
  await Promise.all([store.fetchAll(), suppliers.fetchAll()])
})

async function load() {
  await Promise.all([store.fetchAll(), suppliers.fetchAll()])
}

async function createPurchase() {
  try {
    const payload = { ...form.value }
    if (payload.purchaseDate) {
      payload.purchaseDate = Math.floor(new Date(payload.purchaseDate + 'T12:00:00').getTime() / 1000)
    } else {
      delete payload.purchaseDate
    }
    const data = await store.create(payload)
    showNew.value = false
    router.push('/purchases/' + data.id)
  } catch (e) {
    msg.value = e.message
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
.badge { padding: 2px 8px; border-radius: 12px; font-size: 0.8em; background: #eee; color: #333; }
.badge.draft { background: #eceff1; color: #455a64; }
.badge.validated { background: #e8f5e9; color: #2e7d32; }
.error { color: #c62828; }
.muted { color: #888; }
</style>
