import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch, jsonHeaders, readError } from '../lib/api'

export const useInventoryStore = defineStore('inventory', () => {
  const products = ref([])
  const categories = ref([])
  const brands = ref([])
  const catalogVersion = ref(0)
  const loading = ref(false)
  const error = ref('')

  async function fetchProducts() {
    loading.value = true
    error.value = ''
    try {
      const res = await apiFetch('/api/products')
      if (!res.ok) throw new Error(await readError(res, 'Failed to fetch products'))
      const data = await res.json()
      products.value = data.products || []
      if (data.catalogVersion !== undefined) catalogVersion.value = data.catalogVersion
    } catch (e) {
      error.value = e.message
    } finally {
      loading.value = false
    }
  }

  async function fetchCategories() {
    try {
      const res = await apiFetch('/api/categories')
      if (!res.ok) throw new Error('Failed to fetch categories')
      const data = await res.json()
      categories.value = data.categories || []
    } catch (e) {
      error.value = e.message
    }
  }

  async function fetchBrands() {
    try {
      const res = await apiFetch('/api/brands')
      if (!res.ok) throw new Error('Failed to fetch brands')
      const data = await res.json()
      brands.value = data.brands || []
    } catch (e) {
      error.value = e.message
    }
  }

  async function createProduct(product) {
    const res = await apiFetch('/api/products', {
      method: 'POST',
      headers: jsonHeaders(),
      body: JSON.stringify(payload),
    })
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error || 'Failed to create product')
    }
    await fetchProducts()
  }

  async function updateProduct(sku, updates) {
    const res = await apiFetch(`/api/products/manage?sku=${encodeURIComponent(sku)}`, {
      method: 'PUT',
      headers: jsonHeaders(),
      body: JSON.stringify(updates),
    })
    if (res.status === 409) { const d = await res.json().catch(()=>({})); const e = new Error(d.error||'Conflict'); e.conflict=true; e.current=d; throw e }
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error || 'Failed to update product')
    }
    await fetchProducts()
  }

  async function deleteProduct(sku) {
    const res = await apiFetch(`/api/products/manage?sku=${encodeURIComponent(sku)}`, { method: 'DELETE' })
    if (!res.ok) throw new Error('Failed to archive product')
    await fetchProducts()
  }

  async function createCategory(name) {
    const res = await apiFetch('/api/categories', {
      method: 'POST',
      headers: jsonHeaders(),
      body: JSON.stringify({ name }),
    })
    if (!res.ok) throw new Error('Failed to create category')
    await fetchCategories()
  }

  async function deleteCategory(id) {
    const res = await apiFetch(`/api/categories/manage?id=${encodeURIComponent(id)}`, { method: 'DELETE' })
    if (!res.ok) throw new Error('Failed to delete category')
    await fetchCategories()
  }

  async function createBrand(name) {
    const res = await apiFetch('/api/brands', {
      method: 'POST',
      headers: jsonHeaders(),
      body: JSON.stringify({ name }),
    })
    if (!res.ok) throw new Error('Failed to create brand')
    await fetchBrands()
  }

  async function deleteBrand(id) {
    const res = await apiFetch(`/api/brands/manage?id=${encodeURIComponent(id)}`, { method: 'DELETE' })
    if (!res.ok) throw new Error('Failed to delete brand')
    await fetchBrands()
  }

  async function adjustStock(sku, change, baseStock) {
    const body = { sku, change }
    if (baseStock !== undefined && baseStock !== null) body.baseStock = baseStock
    const res = await apiFetch('/api/stock', {
      method: 'POST',
      headers: jsonHeaders(),
      body: JSON.stringify(body),
    })
    if (res.status === 409) { const d = await res.json().catch(()=>({})); const e = new Error(d.error||'Conflict: stock changed'); e.conflict=true; e.current=d; throw e }
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error || 'Failed to adjust stock')
    }
    await fetchProducts()
  }

  return {
    products, categories, brands, catalogVersion, loading, error,
    fetchProducts, fetchCategories, fetchBrands,
    createProduct, updateProduct, deleteProduct,
    createCategory, deleteCategory,
    createBrand, deleteBrand,
    adjustStock,
  }
})