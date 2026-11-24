<template>
  <div class="view">
    <div class="topbar">
      <button @click="$router.push('/inventory')">← Inventory</button>
      <button @click="load">Refresh</button>
    </div>
    <h1>Needs pricing</h1>
    <p class="muted">{{ unpriced.length }} product(s) without a retail price. Setting a pin makes them sellable.</p>
    <p v-if="store.error && unpriced.length" class="error">{{ friendlyMessage(store.error) }}</p>
    <p v-if="store.loading && !unpriced.length" class="empty-state">{{ COPY.empty.checkingPrices }}</p>

    <ul v-if="unpriced.length" class="list">
      <li v-for="p in unpriced" :key="p.sku" class="card">
        <div class="line-main">
          <strong>{{ p.sku }}</strong> — {{ p.name }}
          <span class="badge" :class="p.active ? 'active' : 'draft'">{{ p.active ? 'Active' : 'Draft' }}</span>
        </div>
        <p class="muted">Base sale price: {{ p.salePrice || '0.00' }} DH — stock: {{ p.stock }}</p>
        <div class="row">
          <input v-model="draft[p.sku]" type="number" step="0.01" min="0" placeholder="Pinned retail price" aria-label="Pinned retail price" />
          <button @click="save(p)">Set pin</button>
          <button v-if="p.pinnedPrice" @click="clear(p)">Clear pin</button>
        </div>
        <p v-if="msgs[p.sku]" class="msg">{{ msgs[p.sku] }}</p>
      </li>
    </ul>
    <p v-else-if="store.error" class="empty-state">{{ friendlyMessage(store.error) }} <router-link to="/pair">{{ COPY.connectToComputer }}</router-link></p>
    <p v-else class="empty-state">{{ COPY.empty.needsPricing }} <router-link to="/catalog">Back to catalog</router-link></p>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useInventoryStore } from '../stores/inventory'
import { COPY, friendlyMessage } from '../lib/copy'

const store = useInventoryStore()
const draft = ref({})
const msgs = ref({})
const unpriced = computed(() => store.products.filter(p => !p.priced))

onMounted(load)

async function load() {
  await store.fetchProducts()
}

async function save(p) {
  const raw = draft.value[p.sku]
  // Blank and 0 both mean "clear": a deliberate zero retail price is not supported.
  const value = raw === undefined || String(raw).trim() === '' || Number(raw) === 0 ? '' : raw
  msgs.value[p.sku] = ''
  try {
    await store.setPinnedPrice({ sku: p.sku, pinnedPrice: value })
    draft.value[p.sku] = ''
    msgs.value[p.sku] = value ? 'Pinned price saved.' : 'Pin cleared.'
  } catch (e) {
    msgs.value[p.sku] = 'Error: ' + e.message
  }
}

async function clear(p) {
  draft.value[p.sku] = ''
  await save(p)
}
</script>

<style scoped>
.topbar { display: flex; justify-content: space-between; margin-bottom: 12px; }
.list { list-style: none; padding: 0; }
.card { border: 1px solid #ddd; border-radius: 8px; padding: 12px; margin-bottom: 8px; }
.line-main { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.row { display: flex; gap: 8px; flex-wrap: wrap; }
.row input { flex: 1; min-width: 140px; padding: 8px; }
.badge { padding: 2px 8px; border-radius: 12px; font-size: 0.8em; background: #eee; }
.badge.active { background: #e8f5e9; color: #2e7d32; }
.badge.draft { background: #fff3e0; color: #e65100; }
.msg { color: #2e7d32; font-size: 0.9em; }
.error { color: #c62828; }
</style>
