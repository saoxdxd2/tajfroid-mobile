<template>
  <div class="view">
    <h1>Catalog</h1>
    <p class="muted">Snapshot: {{ snapshotAt || 'not loaded' }}</p>
    <input v-model="q" placeholder="Search reference / name" />
    <ul>
      <li v-for="p in filtered" :key="p.sku">
        {{ p.sku }} — {{ p.name }} ({{ p.sale_price }} DH) — stock: {{ p.stock }}
      </li>
    </ul>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { useCatalogStore } from '../stores/catalog'
const store = useCatalogStore()
const q = ref('')
const filtered = computed(() => {
  const term = q.value.toLowerCase()
  if (!term) return store.products
  return store.products.filter(p =>
    (p.sku + ' ' + p.name).toLowerCase().includes(term))
})
const snapshotAt = computed(() => store.snapshotAt)
</script>