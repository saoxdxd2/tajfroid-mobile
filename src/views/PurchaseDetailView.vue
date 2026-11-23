<template>
  <div class="view">
    <div class="topbar">
      <button @click="$router.push('/purchases')">← Purchases</button>
      <button @click="load">Refresh</button>
    </div>

    <p v-if="store.error" class="error">{{ store.error }}</p>
    <p v-if="store.loading && !purchase">Loading...</p>

    <template v-if="purchase">
      <div class="card">
        <div class="header-row">
          <h1>{{ purchase.reference }}</h1>
          <span class="badge" :class="purchase.state === 'Validated' ? 'validated' : 'draft'">
            {{ purchase.state }}
          </span>
        </div>
        <p class="muted">{{ purchase.supplierName || '—' }}</p>
        <p v-if="purchase.supplierInvoiceNo" class="muted">Supplier invoice: {{ purchase.supplierInvoiceNo }}</p>
        <p v-if="purchase.purchaseDate" class="muted">Date: {{ formatDate(purchase.purchaseDate) }}</p>
        <div class="total-row">
          <span>Total</span><strong>{{ purchase.totalAmount }} DH</strong>
        </div>
      </div>

      <h3>Items</h3>
      <ul class="line-list">
        <li v-for="it in purchase.items" :key="it.id" class="line">
          <div class="line-main">
            <strong>{{ it.productName || it.productSku }}</strong>
            <span>{{ lineTotal(it) }} DH</span>
          </div>
          <div class="line-meta muted">
            <span>{{ it.quantity }} × {{ it.unitCost }} DH</span>
            <span>{{ it.productSku }}</span>
            <button v-if="purchase.state === 'Draft'" @click="removeItem(it.id)" class="danger small">Remove</button>
          </div>
        </li>
      </ul>

      <div v-if="purchase.state === 'Draft'" class="form-card">
        <h3>Add item</h3>
        <label>Product</label>
        <select v-model="itemForm.productSku" @change="fillFromProduct">
          <option value="">-- Select a product --</option>
          <option v-for="p in inventory.products" :key="p.sku" :value="p.sku">
            {{ p.sku }} — {{ p.name }} (stock {{ p.stock }})
          </option>
        </select>
        <label>Quantity</label>
        <input v-model.number="itemForm.quantity" type="number" min="1" />
        <label>Unit cost (DH)</label>
        <input v-model="itemForm.unitCost" type="number" step="0.01" />
        <button @click="addItem" :disabled="!itemForm.productSku || itemForm.quantity < 1">Add line</button>
        <p v-if="itemMsg" class="error">{{ itemMsg }}</p>
      </div>

      <div class="actions">
        <button v-if="purchase.state === 'Draft'" @click="validatePurchase" class="primary">Validate (add to stock)</button>
        <button v-if="purchase.state === 'Draft'" @click="deletePurchase" class="danger">Delete draft</button>
      </div>
      <p v-if="actionMsg" class="error">{{ actionMsg }}</p>
    </template>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { usePurchaseStore } from '../stores/purchases'
import { useInventoryStore } from '../stores/inventory'

const route = useRoute()
const router = useRouter()
const store = usePurchaseStore()
const inventory = useInventoryStore()
const purchase = computed(() => store.current)
const itemMsg = ref('')
const actionMsg = ref('')
const itemForm = ref({ productSku: '', quantity: 1, unitCost: '' })

onMounted(load)

async function load() {
  await Promise.all([store.fetchOne(route.params.id), inventory.fetchProducts()])
}

function lineTotal(it) {
  return (Number(it.quantity) * Number(it.unitCost)).toFixed(2)
}

function formatDate(ts) {
  const n = Number(ts)
  return n ? new Date(n * 1000).toLocaleDateString() : ''
}

function fillFromProduct() {
  const p = inventory.products.find(x => x.sku === itemForm.value.productSku)
  if (p) itemForm.value.unitCost = p.purchasePrice
}

async function addItem() {
  itemMsg.value = ''
  try {
    await store.addItem(route.params.id, { ...itemForm.value })
    itemForm.value = { productSku: '', quantity: 1, unitCost: '' }
  } catch (e) {
    itemMsg.value = e.message
  }
}

async function removeItem(id) {
  if (!confirm('Remove this line?')) return
  try {
    await store.removeItem(route.params.id, id)
  } catch (e) {
    actionMsg.value = e.message
  }
}

async function validatePurchase() {
  if (!confirm('Validate this purchase? Stock will be increased.')) return
  actionMsg.value = ''
  try {
    await store.validate(route.params.id)
  } catch (e) {
    actionMsg.value = e.message
  }
}

async function deletePurchase() {
  if (!confirm('Delete this draft purchase?')) return
  try {
    await store.remove(route.params.id)
    router.push('/purchases')
  } catch (e) {
    actionMsg.value = e.message
  }
}
</script>

<style scoped>
.topbar { display: flex; justify-content: space-between; margin-bottom: 12px; }
.card { border: 1px solid #ddd; border-radius: 10px; padding: 14px; margin-bottom: 14px; }
.header-row { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.header-row h1 { font-size: 1.2em; margin: 0; }
.total-row { display: flex; justify-content: space-between; margin-top: 12px; padding-top: 10px; border-top: 1px solid #eee; }
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
.badge.validated { background: #e8f5e9; color: #2e7d32; }
.muted { color: #888; }
.error { color: #c62828; }
</style>
