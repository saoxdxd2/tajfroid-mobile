import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch } from '../lib/api'

// The relay catalog uses snake_case while the phone product endpoints have
// historically used camelCase. Normalize both shapes to one product object so
// every surface can rely on effectivePrice / pinnedPrice / priced.
function decimal(value) {
  if (value === null || value === undefined) return ''
  const text = String(value).trim()
  if (!text) return ''
  const n = Number(text)
  if (!Number.isFinite(n) || n <= 0) return ''
  return n.toFixed(2)
}

function pick(...values) {
  return values.find(v => v !== null && v !== undefined && String(v).trim() !== '')
}

export function normalizeProduct(raw) {
  const source = raw && typeof raw === 'object' ? raw : {}
  const salePrice = decimal(pick(source.salePrice, source.sale_price))
  const pinnedPrice = decimal(pick(source.pinnedPrice, source.pinned_price))
  // A missing effective price is derived, never left to a sales surface to
  // guess: a pin wins over sale_price and 0 means unpriced.
  const effectivePrice = decimal(pick(source.effectivePrice, source.effective_price)) || pinnedPrice || salePrice
  const vatRate = source.vatRate ?? source.vat_rate ?? '20'
  const active = source.active === true || source.active === 1 || source.active === '1'
  return {
    ...source,
    sku: String(source.sku ?? ''),
    name: String(source.name ?? ''),
    salePrice,
    pinnedPrice,
    effectivePrice,
    priced: Number(effectivePrice) > 0,
    vatRate,
    stock: Number(source.stock) || 0,
    active,
    // Contract aliases: payloads on the relay use snake_case.
    sale_price: salePrice,
    pinned_price: pinnedPrice,
    effective_price: effectivePrice,
    vat_rate: vatRate,
  }
}

export const useCatalogStore = defineStore('catalog', () => {
  const products = ref([])
  const version = ref(0)
  const snapshotAt = ref('')

  // Pricing surfaces need the full snapshot to count/report unpriced products,
  // then hide them locally. include_unpriced=1 also disables the 304 branch so
  // switching modes cannot serve a stale filtered response.
  async function refresh(includeUnpriced = true) {
    const flag = includeUnpriced ? '&include_unpriced=1' : ''
    const res = await apiFetch(`/api/catalog?version=${version.value}${flag}`)
    if (!includeUnpriced && res.status === 304) return
    if (!res.ok) throw new Error(`catalog refresh failed (${res.status})`)
    const data = await res.json()
    products.value = (data.products || []).map(normalizeProduct)
    version.value = data.version ?? version.value
    snapshotAt.value = data.snapshotAt || new Date().toISOString()
  }

  return { products, version, snapshotAt, refresh, normalizeProduct }
})
