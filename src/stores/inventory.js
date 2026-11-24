import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch, readConflictResponse, jsonHeaders, readError } from '../lib/api'
import { useMutationStore } from './mutations'
import { normalizeProduct } from './catalog'

// Pin writes: omitted/null preserves, "" clears, a positive decimal sets.
function pinValue(value) {
  if (value === undefined || value === null) return null
  if (typeof value === 'number') {
    if (!Number.isFinite(value) || value < 0) throw new Error('Invalid pinned price')
    return value === 0 ? '' : value.toFixed(2)
  }
  const text = String(value).trim()
  if (!text) return ''
  if (!/^\d+(\.\d{1,2})?$/.test(text)) throw new Error('Invalid pinned price')
  const n = Number(text)
  if (!Number.isFinite(n) || n < 0) throw new Error('Invalid pinned price')
  return n === 0 ? '' : n.toFixed(2)
}

export const useInventoryStore = defineStore('inventory', () => {
  const products = ref([])
  const categories = ref([])
  const brands = ref([])
  const catalogVersion = ref(0)
  const loading = ref(false)
  const error = ref('')

  // Pricing surfaces need every product, including imported stock with no
  // price. Sales surfaces filter on `priced` locally.
  async function fetchProducts() {
    loading.value = true
    error.value = ''
    try {
      const res = await apiFetch('/api/products?include_unpriced=1')
      if (!res.ok) throw new Error(await readError(res, 'Failed to fetch products'))
      const data = await res.json()
      products.value = (data.products || []).map(normalizeProduct)
      if (data.catalogVersion !== undefined) catalogVersion.value = data.catalogVersion
    } catch (e) {
      error.value = e.message
    } finally {
      loading.value = false
    }
  }

  function findProduct(sku) {
    return products.value.find(p => p && p.sku === sku)
  }

  function pinState(sku) {
    const p = findProduct(sku)
    if (!p) return null
    return {
      pinnedPrice: p.pinnedPrice, pinned_price: p.pinned_price,
      effectivePrice: p.effectivePrice, effective_price: p.effective_price,
      priced: p.priced, active: p.active,
    }
  }

  function applyPinState(sku, state) {
    const p = findProduct(sku)
    if (p && state) Object.assign(p, state)
  }

  function applyPin(sku, pinnedPrice) {
    const product = findProduct(sku)
    if (!product) return
    product.pinnedPrice = pinnedPrice
    product.pinned_price = pinnedPrice
    product.effectivePrice = pinnedPrice || product.salePrice || ''
    product.effective_price = product.effectivePrice
    product.priced = Number(product.effectivePrice) > 0
    if (pinnedPrice) product.active = true
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
    const body = { ...product }
    try {
      const res = await apiFetch('/api/products', {
        method: 'POST',
        headers: jsonHeaders(),
        body: JSON.stringify(body),
      })
      const conflict = await readConflictResponse(res)
      if (conflict?.duplicate) {
        await fetchProducts()
        return { sku: body.sku, idempotent: true }
      }
      if (!res.ok) {
        throw new Error(conflict?.error || 'Failed to create product')
      }
    } catch (e) {
      if (!navigator.onLine || e instanceof TypeError) {
        await useMutationStore().enqueue({ method: 'POST', path: '/api/products', body, entity: 'product', entityId: body.sku })
        return
      }
      throw e
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
      if (res.status === 409) await useMutationStore().conflictFromResponse(res, { method:'PUT', path:`/api/products/manage?sku=${encodeURIComponent(sku)}`, body, entity:'product', entityId: sku, baseUpdatedAt: body.baseCatalogVersion, fallback: 'Conflict' })
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

  // A pin is a phone-side catalog mutation: one UUID per edit (also used as the
  // queue idempotency key), baseCatalogVersion for optimistic concurrency, and
  // entity 'product' so the existing conflict UI read path can resolve it.
  async function setPinnedPrice({ sku, pinnedPrice }) {
    const value = pinValue(pinnedPrice)
    if (value === null) return // preserve: caller omitted the price
    const id = crypto.randomUUID()
    const path = `/api/products/manage?sku=${encodeURIComponent(sku)}`
    const body = () => ({ id, pinnedPrice: value, baseCatalogVersion: catalogVersion.value })
    const mutations = useMutationStore()
    const previous = pinState(sku)
    applyPin(sku, value) // optimistic: visible immediately, also while offline

    async function resolveConflict(res) {
      // A 409 means the catalog moved; never replay on the stale token.
      await fetchProducts()
      applyPin(sku, value)
      const freshBody = body()
      try {
        await mutations.conflictFromResponse(res, {
          method: 'PUT', path, body: freshBody, entity: 'product', entityId: sku,
          baseUpdatedAt: freshBody.baseCatalogVersion, fallback: 'Conflict', idempotencyKey: id,
        })
      } catch (e) {
        if (!e.conflict) throw e
      }
      let conflictId = mutations.items.find(x => x && x.idempotencyKey === id)?.id
      if (!conflictId) {
        await mutations.load()
        conflictId = mutations.items.find(x => x && x.idempotencyKey === id)?.id
      }
      if (!conflictId) {
        conflictId = await mutations.enqueue({
          method: 'PUT', path, body: freshBody, entity: 'product', entityId: sku,
          baseUpdatedAt: freshBody.baseCatalogVersion, idempotencyKey: id,
        })
      }
      await mutations.updateBody(conflictId, freshBody)
      await mutations.retry(conflictId)
      await mutations.flush()
    }

    try {
      const res = await apiFetch(path, {
        method: 'PUT',
        headers: jsonHeaders(),
        body: JSON.stringify(body()),
      })
      if (res.status === 409) {
        await resolveConflict(res)
        return
      }
      if (!res.ok) throw new Error(await readError(res, 'Failed to set pinned price'))
    } catch (e) {
      if (e.conflict) throw e
      if (!navigator.onLine || e instanceof TypeError) {
        await mutations.enqueue({
          method: 'PUT', path, body: body(), entity: 'product', entityId: sku,
          baseUpdatedAt: catalogVersion.value, idempotencyKey: id,
        })
        return // keep the optimistic state: the edit is queued for replay
      }
      applyPinState(sku, previous) // rejected write: do not show an unsaved pin
      throw e
    }
    await fetchProducts()
  }

  async function deleteProduct(sku, baseCatalogVersion = catalogVersion.value) {
    const q = baseCatalogVersion ? `&baseCatalogVersion=${encodeURIComponent(baseCatalogVersion)}` : ''
    try {
      const res = await apiFetch(`/api/products/manage?sku=${encodeURIComponent(sku)}${q}`, { method: 'DELETE' })
      if (res.status === 409) await useMutationStore().conflictFromResponse(res, { method:'DELETE', path:`/api/products/manage?sku=${encodeURIComponent(sku)}${q}`, body:null, entity:'product', entityId: sku, baseUpdatedAt: baseCatalogVersion, fallback: 'Conflict' })
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
      if (res.status === 409) await useMutationStore().conflictFromResponse(res, { method:'DELETE', path:`/api/categories/manage?id=${encodeURIComponent(id)}${q}`, body:null, entity:'category', entityId:id, baseUpdatedAt, fallback: 'Conflict' })
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
      if (res.status === 409) await useMutationStore().conflictFromResponse(res, { method:'DELETE', path:`/api/brands/manage?id=${encodeURIComponent(id)}${q}`, body:null, entity:'brand', entityId:id, baseUpdatedAt, fallback: 'Conflict' })
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
    try {
      const res = await apiFetch('/api/stock', {
        method: 'POST',
        headers: jsonHeaders(),
        body: JSON.stringify(body),
      })
      if (res.status === 409) await useMutationStore().conflictFromResponse(res, { method:'POST', path:'/api/stock', body, entity:'stock', entityId: sku, baseUpdatedAt: baseStock, fallback: 'Conflict: stock changed' })
      if (!res.ok) {
        const d = await res.json().catch(() => ({}))
        throw new Error(d.error || 'Failed to adjust stock')
      }
    } catch (e) {
      if (!e.conflict && (!navigator.onLine || e instanceof TypeError)) {
        await useMutationStore().enqueue({ method:'POST', path:'/api/stock', body, entity:'stock', entityId: sku, baseUpdatedAt: baseStock })
        return
      }
      throw e
    }
    await fetchProducts()
  }

  return {
    products, categories, brands, catalogVersion, loading, error,
    fetchProducts, fetchCategories, fetchBrands,
    createProduct, updateProduct, deleteProduct, setPinnedPrice,
    createCategory, deleteCategory,
    createBrand, deleteBrand,
    adjustStock,
  }
})