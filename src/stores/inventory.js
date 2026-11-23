import { defineStore } from 'pinia'
import { ref } from 'vue'

const BASE = () => {
  // Will be set by the app
  return localStorage.getItem('relayUrl') || ''
}

function authHeaders() {
  const token = localStorage.getItem('deviceToken') || ''
  return token ? { Authorization: `Bearer ${token}` } : {}
}

export const useInventoryStore = defineStore('inventory', () => {
  const products = ref([])
  const categories = ref([])
  const brands = ref([])
  const loading = ref(false)
  const error = ref('')

  async function fetchProducts() {
    loading.value = true
    error.value = ''
    try {
      const res = await fetch(`${BASE()}/api/products`, { headers: authHeaders() })
      if (!res.ok) throw new Error('Failed to fetch products')
      const data = await res.json()
      products.value = data.products || []
    } catch (e) {
      error.value = e.message
    } finally {
      loading.value = false
    }
  }

  async function fetchCategories() {
    try {
      const res = await fetch(`${BASE()}/api/categories`, { headers: authHeaders() })
      if (!res.ok) throw new Error('Failed to fetch categories')
      const data = await res.json()
      categories.value = data.categories || []
    } catch (e) {
      error.value = e.message
    }
  }

  async function fetchBrands() {
    try {
      const res = await fetch(`${BASE()}/api/brands`, { headers: authHeaders() })
      if (!res.ok) throw new Error('Failed to fetch brands')
      const data = await res.json()
      brands.value = data.brands || []
    } catch (e) {
      error.value = e.message
    }
  }

  async function createProduct(product) {
    const res = await fetch(`${BASE()}/api/products`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...authHeaders() },
      body: JSON.stringify(product),
    })
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error || 'Failed to create product')
    }
    await fetchProducts()
  }

  async function updateProduct(sku, updates) {
    const res = await fetch(`${BASE()}/api/products/manage?sku=${encodeURIComponent(sku)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', ...authHeaders() },
      body: JSON.stringify(updates),
    })
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error || 'Failed to update product')
    }
    await fetchProducts()
  }

  async function deleteProduct(sku) {
    const res = await fetch(`${BASE()}/api/products/manage?sku=${encodeURIComponent(sku)}`, {
      method: 'DELETE',
      headers: authHeaders(),
    })
    if (!res.ok) throw new Error('Failed to archive product')
    await fetchProducts()
  }

  async function createCategory(name) {
    const res = await fetch(`${BASE()}/api/categories`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...authHeaders() },
      body: JSON.stringify({ name }),
    })
    if (!res.ok) throw new Error('Failed to create category')
    await fetchCategories()
  }

  async function deleteCategory(id) {
    const res = await fetch(`${BASE()}/api/categories/manage?id=${encodeURIComponent(id)}`, {
      method: 'DELETE',
      headers: authHeaders(),
    })
    if (!res.ok) throw new Error('Failed to delete category')
    await fetchCategories()
  }

  async function createBrand(name) {
    const res = await fetch(`${BASE()}/api/brands`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...authHeaders() },
      body: JSON.stringify({ name }),
    })
    if (!res.ok) throw new Error('Failed to create brand')
    await fetchBrands()
  }

  async function deleteBrand(id) {
    const res = await fetch(`${BASE()}/api/brands/manage?id=${encodeURIComponent(id)}`, {
      method: 'DELETE',
      headers: authHeaders(),
    })
    if (!res.ok) throw new Error('Failed to delete brand')
    await fetchBrands()
  }

  async function adjustStock(sku, change) {
    const res = await fetch(`${BASE()}/api/stock`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', ...authHeaders() },
      body: JSON.stringify({ sku, change }),
    })
    if (!res.ok) {
      const d = await res.json().catch(() => ({}))
      throw new Error(d.error || 'Failed to adjust stock')
    }
    await fetchProducts()
  }

  return {
    products, categories, brands, loading, error,
    fetchProducts, fetchCategories, fetchBrands,
    createProduct, updateProduct, deleteProduct,
    createCategory, deleteCategory,
    createBrand, deleteBrand,
    adjustStock,
  }
})