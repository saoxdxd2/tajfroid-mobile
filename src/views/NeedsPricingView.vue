<template>
  <div class="space-y-3">
    <div class="flex items-center justify-between gap-2">
      <button type="button" @click="$router.push('/inventory')" class="px-3 py-2 rounded-xl bg-white border border-slate-200 text-slate-700 text-xs font-semibold">
        ← Stock
      </button>
      <button type="button" @click="load" class="px-3 py-2 rounded-xl bg-white border border-slate-200 text-slate-700 text-xs font-semibold">
        Refresh
      </button>
    </div>

    <div>
      <h1 class="text-xl font-bold text-slate-900 m-0">Needs Pricing</h1>
      <p class="text-xs text-slate-500 mt-0.5">{{ unpriced.length }} product(s) currently show "No price available". Pin a retail price to activate.</p>
    </div>

    <p v-if="store.error && unpriced.length" class="text-xs text-rose-700 bg-rose-50 border border-rose-200 rounded-xl p-3">{{ friendlyMessage(store.error) }}</p>
    <p v-if="store.loading && !unpriced.length" class="empty-state">{{ COPY.empty.checkingPrices }}</p>

    <ul v-if="unpriced.length" class="space-y-2.5 list-none p-0 m-0">
      <li
        v-for="p in unpriced"
        :key="p.sku"
        class="bg-white rounded-2xl border p-3.5 shadow-xs space-y-2"
        :class="p.noSku ? 'border-rose-300' : 'border-slate-200'"
      >
        <div class="flex items-center flex-wrap gap-1.5">
          <span class="font-mono text-xs font-bold px-2 py-0.5 rounded-md"
                :class="p.noSku ? 'bg-rose-600 text-white' : 'bg-slate-100 text-slate-800'">
            {{ p.displaySku || p.sku }}
          </span>
          <span v-if="p.isReturn" class="px-2 py-0.5 rounded-full bg-amber-100 text-amber-900 text-[11px] font-bold">
            {{ p.returnLabel || 'Return' }}
          </span>
          <span class="px-2 py-0.5 rounded-full bg-slate-100 text-slate-600 text-[11px] font-semibold">
            Stock: {{ p.stock }}
          </span>
        </div>

        <p class="text-sm font-semibold text-slate-900 leading-snug">{{ p.name }}</p>

        <div class="flex items-center gap-2">
          <input
            v-model="draft[p.sku]"
            type="number"
            inputmode="decimal"
            step="0.01"
            min="0"
            placeholder="Retail price (DH)"
            aria-label="Pinned retail price"
            class="flex-1 min-h-[44px] px-3 rounded-xl border border-slate-200 bg-slate-50 text-sm"
          />
          <button type="button" @click="save(p)" class="min-h-[44px] px-4 rounded-xl bg-blue-700 text-white text-xs font-semibold">
            Set Pin
          </button>
          <button v-if="p.pinnedPrice" type="button" @click="clear(p)" class="min-h-[44px] px-3 rounded-xl bg-slate-100 text-slate-700 text-xs font-semibold">
            Clear
          </button>
        </div>

        <p v-if="msgs[p.sku]" class="text-xs font-semibold" :class="msgs[p.sku].startsWith('Error') ? 'text-rose-700' : 'text-emerald-700'">
          {{ msgs[p.sku] }}
        </p>
      </li>
    </ul>

    <p v-else-if="store.error" class="empty-state">
      {{ friendlyMessage(store.error) }} <router-link to="/pair" class="underline font-semibold">{{ COPY.connectToComputer }}</router-link>
    </p>
    <p v-else class="empty-state">
      {{ COPY.empty.needsPricing }} <router-link to="/catalog" class="underline font-semibold">Back to catalog</router-link>
    </p>
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
