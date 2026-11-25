<template>
  <div class="space-y-3">
    <!-- Header + Toolbar -->
    <div class="flex items-center justify-between gap-2">
      <div>
        <h1 class="text-xl font-bold text-slate-900 m-0">Inventory</h1>
        <p class="text-xs text-slate-500 m-0">{{ store.products.length }} SKUs · {{ totalUnits }} units</p>
      </div>
      <div class="flex items-center gap-2">
        <button type="button" @click="load" class="px-3 py-2 rounded-xl bg-white border border-slate-200 text-slate-700 text-xs font-semibold shadow-xs active:scale-95">
          Refresh
        </button>
        <button type="button" @click="showAdd = !showAdd" class="px-3.5 py-2 rounded-xl bg-blue-700 text-white text-xs font-semibold shadow-xs active:scale-95">
          {{ showAdd ? 'Close' : '+ Product' }}
        </button>
      </div>
    </div>

    <!-- Search + Quick Filter Chips -->
    <div class="space-y-2">
      <input
        v-model="q"
        type="search"
        placeholder="Search SKU, name, brand, category…"
        class="w-full min-h-[44px] px-3.5 py-2.5 rounded-2xl bg-white border border-slate-200 text-sm shadow-xs focus:outline-none focus:ring-2 focus:ring-blue-500"
      />
      <div class="flex items-center gap-2 overflow-x-auto pb-1">
        <button
          v-for="tab in filterTabs"
          :key="tab.id"
          type="button"
          @click="activeFilter = tab.id"
          class="shrink-0 px-3 py-1.5 rounded-full text-xs font-semibold border transition"
          :class="activeFilter === tab.id ? 'bg-slate-900 text-white border-slate-900' : 'bg-white text-slate-600 border-slate-200'"
        >
          {{ tab.label }} ({{ tab.count }})
        </button>
      </div>
    </div>

    <!-- Toast -->
    <div v-if="msg" class="px-3.5 py-2.5 rounded-xl text-xs font-semibold flex items-center justify-between"
         :class="msg.startsWith('Error') ? 'bg-rose-50 text-rose-800 border border-rose-200' : 'bg-emerald-50 text-emerald-800 border border-emerald-200'">
      <span>{{ msg }}</span>
      <button type="button" @click="msg = ''" class="text-xs underline ml-2">Dismiss</button>
    </div>

    <!-- Add Product Card -->
    <div v-if="showAdd" class="bg-white rounded-2xl border border-slate-200 p-4 shadow-sm space-y-2.5">
      <h3 class="text-sm font-bold text-slate-900">New Product</h3>
      <input v-model="form.sku" placeholder="SKU / model code" class="w-full min-h-[44px] px-3 rounded-xl border border-slate-200 bg-slate-50 text-sm" />
      <input v-model="form.name" placeholder="Product name" class="w-full min-h-[44px] px-3 rounded-xl border border-slate-200 bg-slate-50 text-sm" />
      <input v-model="form.description" placeholder="Description" class="w-full min-h-[44px] px-3 rounded-xl border border-slate-200 bg-slate-50 text-sm" />
      <div class="grid grid-cols-2 gap-2">
        <input v-model="form.salePrice" type="number" inputmode="decimal" placeholder="Sale price (DH)" class="min-h-[44px] px-3 rounded-xl border border-slate-200 bg-slate-50 text-sm" />
        <input v-model="form.stock" type="number" inputmode="numeric" placeholder="Stock qty" class="min-h-[44px] px-3 rounded-xl border border-slate-200 bg-slate-50 text-sm" />
      </div>
      <div class="grid grid-cols-2 gap-2">
        <select v-model="form.category" class="min-h-[44px] px-3 rounded-xl border border-slate-200 bg-slate-50 text-sm">
          <option value="">-- Category --</option>
          <option v-for="c in store.categories" :key="c.id" :value="c.name">{{ c.name }}</option>
        </select>
        <select v-model="form.brand" class="min-h-[44px] px-3 rounded-xl border border-slate-200 bg-slate-50 text-sm">
          <option value="">-- Brand --</option>
          <option v-for="b in store.brands" :key="b.id" :value="b.name">{{ b.name }}</option>
        </select>
      </div>
      <label class="flex items-center gap-2 text-xs font-medium text-slate-700">
        <input type="checkbox" v-model="form.active" class="w-4 h-4" /> Active in catalog
      </label>
      <button type="button" @click="addProduct" class="w-full min-h-[44px] rounded-xl bg-blue-700 text-white font-semibold text-sm">Save Product</button>
    </div>

    <p v-if="store.loading && !store.products.length" class="text-sm text-slate-500 text-center py-6">Loading inventory…</p>
    <p v-if="store.error" class="text-xs text-rose-700 bg-rose-50 border border-rose-200 rounded-xl p-3">{{ store.error }}</p>

    <!-- Product Cards -->
    <ul class="space-y-2.5 list-none p-0 m-0">
      <li
        v-for="p in filtered"
        :key="p.sku"
        class="bg-white rounded-2xl border p-3.5 shadow-xs space-y-2.5"
        :class="p.noSku ? 'border-rose-300 ring-1 ring-rose-100' : 'border-slate-200'"
      >
        <!-- Urgent NO SKU Strip -->
        <div v-if="p.noSku" class="flex items-center justify-between gap-2 px-2.5 py-1.5 rounded-xl bg-rose-50 border border-rose-200 text-rose-800 text-xs font-semibold">
          <span>NO SKU — Urgent: update model code</span>
          <button type="button" @click="startEdit(p)" class="underline text-rose-900 shrink-0">Update SKU</button>
        </div>

        <div class="flex items-start justify-between gap-2">
          <div class="min-w-0">
            <div class="flex items-center flex-wrap gap-1.5">
              <span class="font-mono text-xs font-bold px-2 py-0.5 rounded-md"
                    :class="p.noSku ? 'bg-rose-600 text-white' : 'bg-slate-100 text-slate-800'">
                {{ p.displaySku || p.sku }}
              </span>
              <span v-if="p.isReturn" class="px-2 py-0.5 rounded-full bg-amber-100 text-amber-900 text-[11px] font-bold">
                {{ p.returnLabel || 'Return' }}
              </span>
              <span v-if="p.pinnedPrice" class="px-2 py-0.5 rounded-full bg-blue-100 text-blue-800 text-[11px] font-semibold">
                Pinned
              </span>
              <span class="px-2 py-0.5 rounded-full text-[11px] font-semibold"
                    :class="p.active ? 'bg-emerald-100 text-emerald-800' : 'bg-slate-100 text-slate-600'">
                {{ p.active ? 'Active' : 'Draft' }}
              </span>
            </div>
            <p class="text-sm font-semibold text-slate-900 mt-1 leading-snug">{{ p.name }}</p>
            <p class="text-xs text-slate-500 mt-0.5">
              <span v-if="p.brand">{{ p.brand }}</span>
              <span v-if="p.brand && p.category"> · </span>
              <span v-if="p.category">{{ p.category }}</span>
            </p>
          </div>

          <div class="text-right shrink-0">
            <div v-if="p.priced" class="text-sm font-bold text-slate-900">{{ p.effectivePrice }} DH</div>
            <div v-else class="text-[11px] font-bold px-2 py-1 rounded-lg bg-amber-50 text-amber-800 border border-amber-200">
              No price available
            </div>
            <div class="text-xs text-slate-500 mt-1">
              Stock: <strong class="text-slate-900">{{ p.stock }}</strong>
              <span v-if="p.isReturn && p.sealedQty !== p.stock" class="block text-[10px] text-amber-700">
                {{ p.sealedQty }} sealed · {{ p.returnQty }} return
              </span>
            </div>
          </div>
        </div>

        <!-- Touch-Friendly Action Row -->
        <div class="flex items-center justify-between gap-2 pt-2 border-t border-slate-100">
          <div class="flex items-center gap-1.5">
            <button type="button" @click="quickAdjust(p, -1)" class="w-10 h-10 rounded-xl bg-slate-100 text-slate-800 font-bold text-base active:scale-95">-1</button>
            <button type="button" @click="quickAdjust(p, 1)" class="w-10 h-10 rounded-xl bg-slate-100 text-slate-800 font-bold text-base active:scale-95">+1</button>
          </div>
          <div class="flex items-center gap-1.5 flex-1 justify-end">
            <input
              v-model="pinDraft[p.sku]"
              type="number"
              inputmode="decimal"
              step="0.01"
              min="0"
              placeholder="Price DH"
              aria-label="Pinned price"
              class="w-24 h-10 px-2.5 rounded-xl border border-slate-200 bg-slate-50 text-xs"
            />
            <button type="button" @click="setPin(p)" class="h-10 px-3 rounded-xl bg-blue-700 text-white text-xs font-semibold active:scale-95">
              Pin
            </button>
            <button type="button" @click="startEdit(p)" class="h-10 px-3 rounded-xl bg-slate-900 text-white text-xs font-semibold active:scale-95">
              Edit
            </button>
          </div>
        </div>
      </li>
    </ul>

    <p v-if="!store.loading && !filtered.length" class="empty-state">{{ COPY.empty.inventory }}</p>

    <!-- Bottom Sheet Edit Modal -->
    <div v-if="editing" class="fixed inset-0 z-50 bg-slate-900/50 backdrop-blur-xs flex items-end" @click.self="editing = null">
      <div class="w-full max-w-xl mx-auto bg-white rounded-t-3xl p-5 pb-8 shadow-2xl space-y-3 max-h-[85vh] overflow-y-auto">
        <div class="flex items-center justify-between">
          <div>
            <h3 class="text-base font-bold text-slate-900">Edit {{ editing.displaySku || editing.sku }}</h3>
            <p v-if="editing.noSku" class="text-xs font-semibold text-rose-700">NO SKU: enter real carton model code below</p>
          </div>
          <button type="button" @click="editing = null" class="px-3 py-1.5 rounded-lg bg-slate-100 text-slate-700 text-xs font-semibold">Close</button>
        </div>

        <label class="block text-xs font-semibold text-slate-600">
          SKU / Model Code
          <input v-model="editForm.sku" class="mt-1 w-full min-h-[44px] px-3 rounded-xl border border-slate-200 bg-slate-50 text-sm font-mono" />
        </label>
        <label class="block text-xs font-semibold text-slate-600">
          Product Name
          <input v-model="editForm.name" class="mt-1 w-full min-h-[44px] px-3 rounded-xl border border-slate-200 bg-slate-50 text-sm" />
        </label>
        <div class="grid grid-cols-2 gap-2">
          <label class="block text-xs font-semibold text-slate-600">
            Base Sale Price (DH)
            <input v-model="editForm.salePrice" type="number" inputmode="decimal" class="mt-1 w-full min-h-[44px] px-3 rounded-xl border border-slate-200 bg-slate-50 text-sm" />
          </label>
          <label class="block text-xs font-semibold text-slate-600">
            Stock Qty
            <input v-model="editForm.stock" type="number" inputmode="numeric" class="mt-1 w-full min-h-[44px] px-3 rounded-xl border border-slate-200 bg-slate-50 text-sm" />
          </label>
        </div>
        <div class="grid grid-cols-2 gap-2">
          <select v-model="editForm.category" class="min-h-[44px] px-3 rounded-xl border border-slate-200 bg-slate-50 text-sm">
            <option value="">-- Category --</option>
            <option v-for="c in store.categories" :key="c.id" :value="c.name">{{ c.name }}</option>
          </select>
          <select v-model="editForm.brand" class="min-h-[44px] px-3 rounded-xl border border-slate-200 bg-slate-50 text-sm">
            <option value="">-- Brand --</option>
            <option v-for="b in store.brands" :key="b.id" :value="b.name">{{ b.name }}</option>
          </select>
        </div>
        <label class="flex items-center gap-2 text-xs font-medium text-slate-700">
          <input type="checkbox" v-model="editForm.active" class="w-4 h-4" /> Active
        </label>

        <div class="flex items-center gap-2 pt-2">
          <button type="button" @click="saveEdit" class="flex-1 min-h-[44px] rounded-xl bg-blue-700 text-white font-semibold text-sm">Save Changes</button>
          <button v-if="editing.pinnedPrice" type="button" @click="clearPin(editing); editing = null" class="px-3 min-h-[44px] rounded-xl bg-amber-100 text-amber-900 font-semibold text-xs">Clear Pin</button>
          <button type="button" @click="confirmArchiveSku = editing.sku" class="px-3 min-h-[44px] rounded-xl bg-rose-50 text-rose-700 font-semibold text-xs border border-rose-200">Archive</button>
        </div>

        <div v-if="confirmArchiveSku === editing.sku" class="p-3 rounded-xl bg-rose-50 border border-rose-200 flex items-center justify-between gap-2">
          <span class="text-xs font-semibold text-rose-900">Archive {{ editing.sku }}?</span>
          <div class="flex gap-2">
            <button type="button" @click="confirmArchiveSku = ''" class="px-2.5 py-1 rounded-lg bg-white text-xs">Cancel</button>
            <button type="button" @click="remove(editing.sku)" class="px-2.5 py-1 rounded-lg bg-rose-600 text-white text-xs font-semibold">Confirm</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useInventoryStore } from '../stores/inventory'
import { COPY } from '../lib/copy'

const route = useRoute()
const store = useInventoryStore()
const q = ref('')
const activeFilter = ref(route.query.filter === 'nosku' ? 'nosku' : 'all')
const showAdd = ref(false)
const msg = ref('')
const editing = ref(null)
const confirmArchiveSku = ref('')
const pinDraft = ref({})

const form = ref({
  sku: '', name: '', description: '', purchasePrice: '', salePrice: '',
  vatRate: '20', stock: '0', category: '', brand: '', active: true,
})
const editForm = ref({})

watch(() => route.query.filter, (val) => {
  if (val === 'nosku') activeFilter.value = 'nosku'
})

const totalUnits = computed(() => store.products.reduce((acc, p) => acc + (Number(p.stock) || 0), 0))
const noSkuList = computed(() => store.products.filter(p => p.noSku))
const unpricedList = computed(() => store.products.filter(p => !p.priced))
const returnList = computed(() => store.products.filter(p => p.isReturn))

const filterTabs = computed(() => [
  { id: 'all', label: 'All', count: store.products.length },
  { id: 'nosku', label: 'NO SKU', count: noSkuList.value.length },
  { id: 'noprice', label: 'No Price', count: unpricedList.value.length },
  { id: 'returns', label: 'Returns', count: returnList.value.length },
])

const filtered = computed(() => {
  const term = q.value.toLowerCase().trim()
  return store.products.filter(p => {
    if (activeFilter.value === 'nosku' && !p.noSku) return false
    if (activeFilter.value === 'noprice' && p.priced) return false
    if (activeFilter.value === 'returns' && !p.isReturn) return false
    if (!term) return true
    return `${p.sku} ${p.displaySku || ''} ${p.name} ${p.category || ''} ${p.brand || ''}`.toLowerCase().includes(term)
  })
})

onMounted(load)

async function load() {
  await Promise.all([
    store.fetchProducts(),
    store.fetchCategories(),
    store.fetchBrands(),
  ])
}

async function setPin(p) {
  const raw = pinDraft.value[p.sku]
  if (raw === undefined || String(raw).trim() === '' || Number(raw) === 0) {
    await clearPin(p)
    return
  }
  msg.value = ''
  try {
    await store.setPinnedPrice({ sku: p.sku, pinnedPrice: raw })
    pinDraft.value[p.sku] = ''
    msg.value = `Pinned price saved for ${p.displaySku || p.sku}.`
  } catch (e) {
    msg.value = 'Error: ' + e.message
  }
}

async function clearPin(p) {
  msg.value = ''
  try {
    await store.setPinnedPrice({ sku: p.sku, pinnedPrice: '' })
    msg.value = `Pin cleared for ${p.displaySku || p.sku}.`
  } catch (e) {
    msg.value = 'Error: ' + e.message
  }
}

async function addProduct() {
  try {
    await store.createProduct({
      sku: form.value.sku,
      name: form.value.name,
      description: form.value.description,
      purchasePrice: form.value.purchasePrice || '0',
      salePrice: form.value.salePrice || '0',
      vatRate: form.value.vatRate || '20',
      stock: parseInt(form.value.stock) || 0,
      active: form.value.active,
      category: form.value.category,
      brand: form.value.brand,
    })
    msg.value = 'Product created!'
    form.value = { sku: '', name: '', description: '', purchasePrice: '', salePrice: '', vatRate: '20', stock: '0', category: '', brand: '', active: true }
    showAdd.value = false
  } catch (e) {
    msg.value = 'Error: ' + e.message
  }
}

function startEdit(p) {
  confirmArchiveSku.value = ''
  editing.value = p
  editForm.value = {
    sku: p.noSku ? '' : p.sku,
    name: p.name, description: p.description || '',
    purchasePrice: p.purchasePrice, salePrice: p.salePrice,
    vatRate: p.vatRate, stock: p.stock,
    category: p.category || '', brand: p.brand || '', active: p.active,
  }
}

async function saveEdit() {
  try {
    await store.updateProduct(editing.value.sku, {
      sku: editForm.value.sku || editing.value.sku,
      name: editForm.value.name,
      description: editForm.value.description,
      purchasePrice: editForm.value.purchasePrice,
      salePrice: editForm.value.salePrice,
      vatRate: editForm.value.vatRate,
      stock: editForm.value.stock,
      active: editing.value.pinnedPrice ? true : editForm.value.active,
      category: editForm.value.category,
      brand: editForm.value.brand,
      baseCatalogVersion: store.catalogVersion || undefined,
    })
    msg.value = `Saved ${editForm.value.sku || editing.value.sku}.`
    editing.value = null
  } catch (e) {
    msg.value = 'Error: ' + e.message
  }
}

async function remove(sku) {
  try {
    await store.deleteProduct(sku, store.catalogVersion)
    confirmArchiveSku.value = ''
    editing.value = null
    msg.value = `Archived ${sku}.`
  } catch (e) {
    msg.value = 'Error: ' + e.message
  }
}

async function quickAdjust(p, delta) {
  try {
    await store.adjustStock(p.sku, delta, p.stock)
    msg.value = `${p.displaySku || p.sku}: stock adjusted by ${delta > 0 ? '+' : ''}${delta}.`
  } catch (e) {
    msg.value = e.conflict ? 'Error: Conflict — stock changed on another device. Refreshing.' : 'Error: ' + e.message
    await store.fetchProducts()
  }
}
</script>
