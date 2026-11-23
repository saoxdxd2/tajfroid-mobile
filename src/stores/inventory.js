import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch, jsonHeaders, readError } from '../lib/api'
import { useMutationStore } from './mutations'

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
    const body = { ...updates }
    try {
      const res = await apiFetch(`/api/products/manage?sku=${encodeURIComponent(sku)}`, {
        method: 'PUT',
        headers: jsonHeaders(),
        body: JSON.stringify(body),
      })
      if (res.status === 409) {
        const d = await res.json().catch(()=>({}))
        await useMutationStore().recordConflict({ method:'PUT', path:`/api/products/manage?sku=${encodeURIComponent(sku)}`, body, entity:'product', entityId: sku, baseUpdatedAt: body.baseCatalogVersion, conflict:d, error:d.error })
        const e = new Error(d.error||'Conflict'); e.conflict=true; e.current=d; throw e
      }
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.error || 'Failed to update product')
      }
    } catch (e) {
      if (!e.conflict && (!navigator.onLine || e instanceof TypeError)) {
        await useMutationStore().enqueue({ method: 'PUT', path: `/api/products/manage?sku=${encodeURIComponent(sku)}`, body, entity: 'product', entityId: sku, baseUpdatedAt: body.baseCatalogVersion })
        return
      }
      throw e
    }
    await fetchProducts()
  }

  async function deleteProduct(sku, baseCatalogVersion = catalogVersion.value) {
    const q = baseCatalogVersion ? `&baseCatalogVersion=${encodeURIComponent(baseCatalogVersion)}` : ''
    try {
      const res = await apiFetch(`/api/products/manage?sku=${encodeURIComponent(sku)}${q}`, { method: 'DELETE' })
      if (res.status === 409) {
        const d = await res.json().catch(()=>({}))
        await useMutationStore().recordConflict({ method:'DELETE', path:`/api/products/manage?sku=${encodeURIComponent(sku)}${q}`, body:null, entity:'product', entityId: sku, baseUpdatedAt: baseCatalogVersion, conflict:d, error:d.error })
        const e = new Error(d.error||'Conflict'); e.conflict=true; e.current=d; throw e
      }
      if (!res.ok) throw new Error('Failed to archive product')
    } catch (e) {
      if (!e.conflict && (!navigator.onLine || e instanceof TypeError)) {
        await useMutationStore().enqueue({ method: 'DELETE', path: `/api/products/manage?sku=${encodeURIComponent(sku)}${q}`, body: null, entity: 'product', entityId: sku, baseUpdatedAt: baseCatalogVersion })
        return
      }
      throw e
    }
    await fetchProducts()
  }

  async function createCategory(name) {
    const body = { id: crypto.randomUUID(), name }
    try {
      const res = await apiFetch('/api/categories', {
        method: 'POST',
        headers: jsonHeaders(),
        body: JSON.stringify(body),
      })
      if (!res.ok) throw new Error('Failed to create category')
    } catch (e) {
      if (!navigator.onLine || e instanceof TypeError) {
        await useMutationStore().enqueue({ method: 'POST', path: '/api/categories', body, entity: 'category', entityId: body.id })
        categories.value.push({ id: body.id, name, _pending: true })
        return
      }
      throw e
    }
    await fetchCategories()
  }

  async function deleteCategory(id, baseUpdatedAt) {
    const q = baseUpdatedAt ? `&baseUpdatedAt=${encodeURIComponent(baseUpdatedAt)}` : ''
    try {
      const res = await apiFetch(`/api/categories/manage?id=${encodeURIComponent(id)}${q}`, { method: 'DELETE' })
      if (res.status === 409) {
        const d = await res.json().catch(()=>({}))
        await useMutationStore().recordConflict({ method:'DELETE', path:`/api/categories/manage?id=${encodeURIComponent(id)}${q}`, body:null, entity:'category', entityId:id, baseUpdatedAt, conflict:d, error:d.error })
        const e = new Error(d.error||'Conflict'); e.conflict=true; e.current=d; throw e
      }
      if (!res.ok) throw new Error('Failed to delete category')
    } catch (e) {
      if (!e.conflict && (!navigator.onLine || e instanceof TypeError)) {
        await useMutationStore().enqueue({ method: 'DELETE', path: `/api/categories/manage?id=${encodeURIComponent(id)}${q}`, body: null, entity: 'category', entityId: id, baseUpdatedAt })
        return
      }
      throw e
    }
    await fetchCategories()
  }

  async function createBrand(name) {
    const body = { id: crypto.randomUUID(), name }
    try {
      const res = await apiFetch('/api/brands', {
        method: 'POST',
        headers: jsonHeaders(),
        body: JSON.stringify(body),
      })
      if (!res.ok) throw new Error('Failed to create brand')
    } catch (e) {
      if (!navigator.onLine || e instanceof TypeError) {
        await useMutationStore().enqueue({ method: 'POST', path: '/api/brands', body, entity: 'brand', entityId: body.id })
        brands.value.push({ id: body.id, name, _pending: true })
        return
      }
      throw e
    }
    await fetchBrands()
  }

  async function deleteBrand(id, baseUpdatedAt) {
    const q = baseUpdatedAt ? `&baseUpdatedAt=${encodeURIComponent(baseUpdatedAt)}` : ''
    try {
      const res = await apiFetch(`/api/brands/manage?id=${encodeURIComponent(id)}${q}`, { method: 'DELETE' })
      if (res.status === 409) {
        const d = await res.json().catch(()=>({}))
        await useMutationStore().recordConflict({ method:'DELETE', path:`/api/brands/manage?id=${encodeURIComponent(id)}${q}`, body:null, entity:'brand', entityId:id, baseUpdatedAt, conflict:d, error:d.error })
        const e = new Error(d.error||'Conflict'); e.conflict=true; e.current=d; throw e
      }
      if (!res.ok) throw new Error('Failed to delete brand')
    } catch (e) {
      if (!e.conflict && (!navigator.onLine || e instanceof TypeError)) {
        await useMutationStore().enqueue({ method: 'DELETE', path: `/api/brands/manage?id=${encodeURIComponent(id)}${q}`, body: null, entity: 'brand', entityId: id, baseUpdatedAt })
        return
      }
      throw e
    }
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
    if (res.status === 409) {
      const d = await res.json().catch(()=>({}))
      await useMutationStore().recordConflict({ method:'POST', path:'/api/stock', body, entity:'stock', entityId: sku, baseUpdatedAt: baseStock, conflict:d, error:d.error })
      const e = new Error(d.error||'Conflict: stock changed'); e.conflict=true; e.current=d; throw e
    }
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