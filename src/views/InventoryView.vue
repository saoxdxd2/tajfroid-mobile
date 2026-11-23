<template>
  <div class="view">
    <h1>Inventory</h1>
    <div class="toolbar">
      <input v-model="q" placeholder="Search SKU / name" />
      <button @click="showAdd = !showAdd">{{ showAdd ? 'Cancel' : '+ Add product' }}</button>
      <button @click="load">Refresh</button>
    </div>

    <div v-if="showAdd" class="form-card">
      <h3>New product</h3>
      <input v-model="form.sku" placeholder="SKU (référence)" />
      <input v-model="form.name" placeholder="Name" />
      <input v-model="form.description" placeholder="Description" />
      <input v-model="form.purchasePrice" type="number" placeholder="Purchase price" />
      <input v-model="form.salePrice" type="number" placeholder="Sale price" />
      <input v-model="form.vatRate" type="number" placeholder="VAT %" />
      <input v-model="form.stock" type="number" placeholder="Stock qty" />
      <select v-model="form.category">
        <option value="">-- Category --</option>
        <option v-for="c in store.categories" :key="c.id" :value="c.name">{{ c.name }}</option>
      </select>
      <select v-model="form.brand">
        <option value="">-- Brand --</option>
        <option v-for="b in store.brands" :key="b.id" :value="b.name">{{ b.name }}</option>
      </select>
      <label><input type="checkbox" v-model="form.active" /> Active</label>
      <button @click="addProduct">Save</button>
      <p v-if="msg" class="msg">{{ msg }}</p>
    </div>

    <p v-if="store.loading">Loading...</p>
    <p v-if="store.error" class="error">{{ store.error }}</p>

    <ul class="product-list">
      <li v-for="p in filtered" :key="p.sku" class="product-item">
        <div class="product-header">
          <strong>{{ p.sku }}</strong> — {{ p.name }}
          <span class="badge" :class="p.active ? 'active' : 'draft'">{{ p.active ? 'Active' : 'Draft' }}</span>
        </div>
        <div class="product-details">
          <span>Stock: {{ p.stock }}</span>
          <span>Price: {{ p.salePrice }} DH</span>
          <span v-if="p.category">Cat: {{ p.category }}</span>
          <span v-if="p.brand">Brand: {{ p.brand }}</span>
        </div>
        <div class="product-actions">
          <button @click="quickAdjust(p, 1)">+1</button>
          <button @click="quickAdjust(p, -1)">-1</button>
          <button @click="startEdit(p)">Edit</button>
          <button @click="remove(p.sku)" class="danger">Archive</button>
        </div>
      </li>
    </ul>

    <!-- Edit modal -->
    <div v-if="editing" class="modal-overlay" @click.self="editing = null">
      <div class="modal">
        <h3>Edit {{ editing.sku }}</h3>
        <input v-model="editForm.name" placeholder="Name" />
        <input v-model="editForm.description" placeholder="Description" />
        <input v-model="editForm.purchasePrice" type="number" placeholder="Purchase price" />
        <input v-model="editForm.salePrice" type="number" placeholder="Sale price" />
        <input v-model="editForm.vatRate" type="number" placeholder="VAT %" />
        <input v-model="editForm.stock" type="number" placeholder="Stock" />
        <select v-model="editForm.category">
          <option value="">-- Category --</option>
          <option v-for="c in store.categories" :key="c.id" :value="c.name">{{ c.name }}</option>
        </select>
        <select v-model="editForm.brand">
          <option value="">-- Brand --</option>
          <option v-for="b in store.brands" :key="b.id" :value="b.name">{{ b.name }}</option>
        </select>
        <label><input type="checkbox" v-model="editForm.active" /> Active</label>
        <div class="modal-actions">
          <button @click="saveEdit">Save</button>
          <button @click="editing = null">Cancel</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useInventoryStore } from '../stores/inventory'

const store = useInventoryStore()
const q = ref('')
const showAdd = ref(false)
const msg = ref('')
const editing = ref(null)

const form = ref({
  sku: '', name: '', description: '', purchasePrice: '', salePrice: '',
  vatRate: '20', stock: '0', category: '', brand: '', active: true,
})

const editForm = ref({})

const filtered = computed(() => {
  const term = q.value.toLowerCase()
  if (!term) return store.products
  return store.products.filter(p =>
    (p.sku + ' ' + p.name + ' ' + (p.category || '') + ' ' + (p.brand || '')).toLowerCase().includes(term))
})

onMounted(async () => {
  await Promise.all([store.fetchProducts(), store.fetchCategories(), store.fetchBrands()])
})

async function load() {
  await Promise.all([store.fetchProducts(), store.fetchCategories(), store.fetchBrands()])
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
  editing.value = p
  editForm.value = {
    name: p.name, description: p.description || '',
    purchasePrice: p.purchasePrice, salePrice: p.salePrice,
    vatRate: p.vatRate, stock: p.stock,
    category: p.category || '', brand: p.brand || '', active: p.active,
  }
}

async function saveEdit() {
  try {
    await store.updateProduct(editing.value.sku, {
      name: editForm.value.name,
      description: editForm.value.description,
      purchasePrice: editForm.value.purchasePrice,
      salePrice: editForm.value.salePrice,
      vatRate: editForm.value.vatRate,
      stock: editForm.value.stock,
      active: editForm.value.active,
      category: editForm.value.category,
      brand: editForm.value.brand,
    })
    editing.value = null
  } catch (e) {
    alert('Error: ' + e.message)
  }
}

async function remove(sku) {
  if (!confirm('Archive this product?')) return
  try {
    await store.deleteProduct(sku)
  } catch (e) {
    alert('Error: ' + e.message)
  }
}

async function quickAdjust(p, delta) {
  try {
    await store.adjustStock(p.sku, delta, p.stock)
  } catch (e) {
    if (e.conflict) alert('Conflict: stock changed on another device. Refreshing.')
    else alert('Error: ' + e.message)
    await store.fetchProducts()
  }
}
</script>

<style scoped>
.toolbar { display: flex; gap: 8px; margin-bottom: 12px; flex-wrap: wrap; }
.toolbar input { flex: 1; min-width: 150px; padding: 8px; }
.form-card { background: #f5f5f5; padding: 12px; border-radius: 8px; margin-bottom: 12px; }
.form-card input, .form-card select { display: block; width: 100%; margin: 4px 0; padding: 8px; }
.product-list { list-style: none; padding: 0; }
.product-item { border: 1px solid #ddd; border-radius: 8px; padding: 12px; margin-bottom: 8px; }
.product-header { display: flex; justify-content: space-between; align-items: center; }
.product-details { display: flex; gap: 12px; flex-wrap: wrap; font-size: 0.9em; color: #666; margin: 4px 0; }
.product-actions { display: flex; gap: 6px; margin-top: 8px; }
.badge { padding: 2px 8px; border-radius: 12px; font-size: 0.8em; }
.badge.active { background: #e8f5e9; color: #2e7d32; }
.badge.draft { background: #fff3e0; color: #e65100; }
.msg { color: green; }
.error { color: red; }
.modal-overlay { position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.5); display: flex; align-items: center; justify-content: center; z-index: 200; }
.modal { background: #fff; padding: 20px; border-radius: 8px; width: 90%; max-width: 400px; }
.modal input, .modal select { display: block; width: 100%; margin: 4px 0; padding: 8px; }
.modal-actions { display: flex; gap: 8px; margin-top: 12px; }
.danger { color: red; }
</style>