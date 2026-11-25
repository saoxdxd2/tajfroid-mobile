<template>
  <div class="space-y-3">
    <div class="flex items-center justify-between gap-2">
      <div>
        <h1 class="text-xl font-bold text-slate-900 m-0">Catalog</h1>
        <p class="text-xs text-slate-500 m-0">
          {{ filtered.length }} visible · {{ unpricedCount }} unpriced
          <span v-if="snapshotAt"> · Synced</span>
        </p>
      </div>
      <button type="button" @click="reload" class="px-3 py-2 rounded-xl bg-white border border-slate-200 text-slate-700 text-xs font-semibold shadow-xs">
        Sync
      </button>
    </div>

    <input
      v-model="q"
      type="search"
      placeholder="Search SKU, product name, brand…"
      class="w-full min-h-[44px] px-3.5 py-2.5 rounded-2xl bg-white border border-slate-200 text-sm shadow-xs focus:outline-none focus:ring-2 focus:ring-blue-500"
    />

    <p v-if="error" class="text-xs text-rose-700 bg-rose-50 border border-rose-200 rounded-xl p-3">{{ error }}</p>

    <ul class="space-y-2 list-none p-0 m-0">
      <li
        v-for="p in filtered"
        :key="p.sku"
        class="bg-white rounded-2xl border p-3.5 shadow-xs flex items-center justify-between gap-3"
        :class="p.noSku ? 'border-rose-300' : 'border-slate-200'"
      >
        <div class="min-w-0">
          <div class="flex items-center flex-wrap gap-1.5">
            <span class="font-mono text-xs font-bold px-2 py-0.5 rounded-md"
                  :class="p.noSku ? 'bg-rose-600 text-white' : 'bg-slate-100 text-slate-800'">
              {{ p.displaySku || p.sku }}
            </span>
            <span v-if="p.isReturn" class="px-2 py-0.5 rounded-full bg-amber-100 text-amber-900 text-[11px] font-bold">
              {{ p.returnLabel || 'Return' }}
            </span>
          </div>
          <p class="text-sm font-semibold text-slate-900 mt-1 leading-snug">{{ p.name }}</p>
          <p class="text-xs text-slate-500 mt-0.5">{{ [p.brand, p.category].filter(Boolean).join(' · ') }}</p>
        </div>

        <div class="text-right shrink-0">
          <div v-if="p.priced" class="text-sm font-bold text-slate-900">{{ p.effectivePrice }} DH</div>
          <router-link v-else to="/needs-pricing" class="inline-block text-[11px] font-bold px-2 py-1 rounded-lg bg-amber-50 text-amber-800 border border-amber-200">
            No price available
          </router-link>
          <div class="text-xs text-slate-500 mt-1">Qty: <strong class="text-slate-900">{{ p.stock }}</strong></div>
        </div>
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

const unpricedCount = computed(() => store.products.filter(p => !p.priced).length)
const filtered = computed(() => {
  const term = q.value.toLowerCase().trim()
  return store.products.filter(p => {
    if (!term) return true
    return `${p.sku} ${p.displaySku || ''} ${p.name} ${p.brand || ''} ${p.category || ''}`.toLowerCase().includes(term)
  })
})
const snapshotAt = computed(() => store.snapshotAt)

onMounted(reload)

async function reload() {
  error.value = ''
  try {
    await store.refresh(true)
  } catch (e) {
    error.value = e.message
  }
}
</script>
