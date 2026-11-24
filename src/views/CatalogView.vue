<template>
  <div class="view">
    <h1>Catalog</h1>
    <p class="muted">Snapshot: {{ snapshotAt || 'not loaded' }}</p>
    <p v-if="error" class="error">{{ error }}</p>
    <p class="pricing-link">
      <router-link to="/needs-pricing">Needs pricing ({{ unpricedCount }})</router-link>
    </p>
    <input v-model="q" placeholder="Search reference / name" />
    <ul>
      <li v-for="p in filtered" :key="p.sku">
        {{ p.sku }} — {{ p.name }} ({{ p.effectivePrice }} DH) — stock: {{ p.stock }}
      </li>
    </ul>
    <p v-if="!filtered.length" class="empty-state">{{ COPY.empty.catalog }}</p>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useCatalogStore } from '../stores/catalog'
import { COPY } from '../lib/copy'

const store = useCatalogStore()
const q = ref('')
const error = ref('')

// The snapshot includes unpriced stock only so we can report the count;
// sales surfaces never show or sell it.
const unpricedCount = computed(() => store.products.filter(p => !p.priced).length)
const filtered = computed(() => {
  const term = q.value.toLowerCase()
  return store.products
    .filter(p => p.priced)
    .filter(p => !term || (p.sku + ' ' + p.name).toLowerCase().includes(term))
})
const snapshotAt = computed(() => store.snapshotAt)

onMounted(async () => {
  try {
    await store.refresh()
  } catch (e) {
    error.value = e.message
  }
})
</script>

<style scoped>
.pricing-link { font-size: 0.9em; }
.error { color: #c62828; }
</style>
