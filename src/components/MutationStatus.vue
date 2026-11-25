<template>
  <div v-if="items.length || syncing" class="fixed left-3 right-3 bottom-20 z-30 max-w-xl mx-auto rounded-2xl bg-amber-50/95 backdrop-blur border border-amber-300 shadow-lg p-3 text-xs">
    <div class="flex items-center gap-2">
      <span class="w-2 h-2 rounded-full shrink-0" :class="conflicts ? 'bg-rose-600' : 'bg-amber-500 animate-pulse'"></span>
      <strong class="text-amber-950">{{ COPY.mutationTitle }}</strong>
      <span v-if="syncing" class="text-amber-800">{{ COPY.mutationSyncing }}</span>
      <span v-else class="text-amber-800">{{ COPY.mutationCounts(pending, conflicts) }}</span>
      <button type="button" class="ml-auto font-semibold text-blue-800 underline" @click="store.flush()">Sync</button>
      <button type="button" class="font-semibold text-slate-700 underline" @click="open = !open">{{ open ? 'Hide' : 'Details' }}</button>
    </div>

    <div v-if="open" class="mt-2.5 max-h-[42vh] overflow-y-auto space-y-2 border-t border-amber-200 pt-2">
      <p v-if="uiNotice" class="px-2.5 py-1.5 rounded-lg bg-rose-100 text-rose-900 font-semibold">{{ uiNotice }}</p>
      <div v-for="m in items" :key="m.id" class="bg-white rounded-xl border border-amber-200 p-2.5 space-y-1.5">
        <div class="flex items-center gap-2">
          <span class="px-2 py-0.5 rounded-full text-[10px] font-bold uppercase text-white"
                :class="m.status === 'conflict' ? 'bg-rose-600' : 'bg-amber-600'">
            {{ m.status }}
          </span>
          <span class="font-semibold text-slate-800">{{ m.entity || 'mutation' }} {{ m.method }}</span>
          <span class="text-slate-400 ml-auto">#{{ m.attempts || 0 }}</span>
        </div>
        <div class="text-[11px] text-slate-500 break-all">{{ m.path }}</div>
        <div v-if="m.lastError" class="text-[11px] text-rose-700 whitespace-pre-wrap">{{ m.lastError }}</div>

        <div v-if="m.status === 'conflict'" class="flex flex-wrap gap-1.5">
          <button type="button" @click="fetchServer(m)" class="px-2.5 py-1 rounded-lg bg-slate-900 text-white font-semibold">Load server diff</button>
        </div>

        <div v-if="server[m.id]" class="border border-slate-200 rounded-lg p-2 bg-slate-50 space-y-1">
          <div class="grid grid-cols-4 gap-1 font-bold text-[10px] text-slate-600">
            <span>Field</span><span>Local</span><span>Server</span><span>Pick</span>
          </div>
          <div v-for="k in diffKeysByItem[m.id] || []" :key="k" class="grid grid-cols-4 gap-1 items-center border-t border-slate-200 pt-1">
            <span class="font-mono text-[10px]">{{ k }}</span>
            <span class="text-[10px] break-all">{{ formatted[m.id]?.local?.[k] }}</span>
            <span class="text-[10px] break-all">{{ formatted[m.id]?.server?.[k] }}</span>
            <span class="flex gap-1.5 text-[10px]">
              <label><input type="radio" :name="m.id+k" :checked="pickOf(m,k)==='local'" @change="choose(m.id,k,'local')"> L</label>
              <label><input type="radio" :name="m.id+k" :checked="pickOf(m,k)==='server'" @change="choose(m.id,k,'server')"> S</label>
            </span>
          </div>
          <button type="button" @click="applyMerge(m)" class="mt-1 px-2.5 py-1 rounded-lg bg-blue-700 text-white font-semibold">Apply merge</button>
        </div>

        <div class="flex gap-2 pt-1">
          <button v-if="m.status==='conflict'" type="button" @click="retry(m.id)" class="px-2.5 py-1 rounded-lg bg-blue-700 text-white font-semibold">Retry local</button>
          <button v-if="m.status==='conflict'" type="button" @click="dismiss(m.id)" class="px-2.5 py-1 rounded-lg bg-slate-100 text-slate-700 font-semibold">Keep server</button>
          <button v-if="m.status!=='conflict'" type="button" @click="retry(m.id)" class="px-2.5 py-1 rounded-lg bg-blue-700 text-white font-semibold">Retry</button>
          <button v-if="m.status!=='conflict'" type="button" @click="remove(m.id)" class="px-2.5 py-1 rounded-lg bg-slate-100 text-slate-700 font-semibold">Dismiss</button>
        </div>
      </div>
      <p class="text-[11px] text-slate-500 m-0">{{ COPY.mutationHint }}</p>
    </div>
  </div>
</template>

<script setup>
import { computed, reactive, ref, watch } from 'vue'
import { apiFetch } from '../lib/api'
import { COPY } from '../lib/copy'
import { useMutationStore } from '../stores/mutations'

const store = useMutationStore()
const open = ref(false)
const uiNotice = ref('')
const edits = reactive({})
const server = reactive({})
const conflictObj = reactive({})
const mergeSel = reactive({})

// Applied rows stay in IndexedDB as replay guards; only show work needing action.
const items = computed(() => store.items.filter(i => i.status !== 'applied'))
const syncing = computed(() => store.syncing)
const pending = computed(() => items.value.filter(i => i.status === 'pending').length)
const conflicts = computed(() => items.value.filter(i => i.status === 'conflict').length)

function pretty(v) { try { return JSON.stringify(v ?? {}, null, 2) } catch { return String(v) } }

const localObjects = computed(() => {
  const out = {}
  for (const id of Object.keys(edits)) {
    try { out[id] = JSON.parse(edits[id] || '{}') } catch { out[id] = {} }
  }
  return out
})
const serverObjects = computed(() => {
  const out = {}
  for (const id of Object.keys(server)) out[id] = server[id] || {}
  return out
})
const conflictObjects = computed(() => {
  const out = {}
  for (const id of Object.keys(conflictObj)) out[id] = conflictObj[id] || {}
  return out
})

const VALUE_TYPES = ['string', 'number', 'boolean', 'object']
const diffKeysByItem = computed(() => {
  const out = {}
  for (const m of items.value) {
    const l = localObjects.value[m.id] || {}
    const s = serverObjects.value[m.id] || {}
    const c = conflictObjects.value[m.id] || {}
    out[m.id] = Array.from(new Set([...Object.keys(l), ...Object.keys(s), ...Object.keys(c)]))
      .filter(k => VALUE_TYPES.includes(typeof l[k]) || VALUE_TYPES.includes(typeof s[k]) || VALUE_TYPES.includes(typeof c[k]))
  }
  return out
})

const formatted = computed(() => {
  const out = {}
  for (const m of items.value) {
    const id = m.id
    const l = localObjects.value[id] || {}
    const s = serverObjects.value[id] || {}
    const local = {}
    const remote = {}
    for (const k of diffKeysByItem.value[id] || []) {
      local[k] = pretty(l[k])
      remote[k] = pretty(s[k])
    }
    out[id] = { local, server: remote }
  }
  return out
})

watch(items, (list) => {
  for (const m of list) if (edits[m.id] === undefined) edits[m.id] = pretty(m.body)
}, { immediate: true })

function retry(id) { store.retry(id) }
function remove(id) { store.remove(id) }
function dismiss(id) { store.remove(id) }

function readPathFor(m) {
  if (!m?.entityId) return null
  const id = encodeURIComponent(m.entityId)
  const map = {
    customer: `/api/customers/manage?id=${id}`,
    supplier: `/api/suppliers/manage?id=${id}`,
    check: `/api/checks/manage?id=${id}`,
    invoice: `/api/invoices/manage?id=${id}`,
    purchase: `/api/purchases/manage?id=${id}`,
    product: `/api/products/manage?sku=${id}`,
    category: `/api/categories/manage?id=${id}`,
    brand: `/api/brands/manage?id=${id}`,
    expense: `/api/expenses/manage?id=${id}`,
    stock: `/api/stock?sku=${id}`,
    device: `/api/devices/manage?id=${id}`,
  }
  return map[m.entity] || null
}

function seedServerFromConflict(m) {
  const c = m?.conflict || {}
  const candidate = c.current || c.record || c.server || c.currentRecord || c
  if (candidate && typeof candidate === 'object' && !Array.isArray(candidate)) {
    const keys = Object.keys(candidate).filter(k => k !== 'error')
    if (keys.length) { server[m.id] = candidate; conflictObj[m.id] = candidate; return true }
  }
  return false
}

async function fetchServer(m) {
  uiNotice.value = ''
  if (seedServerFromConflict(m)) return
  const p = readPathFor(m)
  if (!p) { uiNotice.value = 'No server read endpoint mapped for this entity.'; return }
  try {
    const res = await apiFetch(p)
    if (!res.ok) throw new Error(await res.text())
    server[m.id] = await res.json()
    if (!conflictObj[m.id]) conflictObj[m.id] = {}
  } catch (e) {
    uiNotice.value = 'Failed to load server record: ' + e.message
  }
}

function pickOf(m, k) {
  const sel = mergeSel[m.id]
  if (sel && sel[k]) return sel[k]
  const l = (localObjects.value[m.id] || {})[k]
  const s = (serverObjects.value[m.id] || {})[k]
  return l === s ? 'server' : 'local'
}
function choose(id, k, side) { mergeSel[id] = mergeSel[id] || {}; mergeSel[id][k] = side }

async function applyMerge(m) {
  const l = localObjects.value[m.id] || {}
  const s = serverObjects.value[m.id] || {}
  const out = { ...l }
  for (const k of diffKeysByItem.value[m.id] || []) {
    out[k] = pickOf(m, k) === 'server' ? s[k] : l[k]
  }
  edits[m.id] = pretty(out)
  await store.updateBody(m.id, out)
  await store.retry(m.id)
}
</script>
