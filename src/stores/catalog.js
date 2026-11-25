import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch } from '../lib/api'

const CATALOG_CACHE_KEY = 'taj_catalog_offline_v1'

// The relay catalog uses snake_case while the phone product endpoints have
// historically used camelCase. Normalize both shapes to one product object so
// every surface can rely on effectivePrice / pinnedPrice / priced + NO SKU / Return flags.
function decimal(value) {
  if (value === null || value === undefined) return ''
  const text = String(value).trim()
  if (!text) return ''
  const n = Number(text)
  if (!Number.isFinite(n) || n <= 0) return ''
  return n.toFixed(2)
}

function pick(...values) {
  return values.find(v => v !== null && valuePresent(v))
}

function valuePresent(v) {
  return v !== undefined && String(v).trim() !== ''
}

function boolFlag(v) {
  return v === true || v === 1 || v === '1'
}

export function normalizeProduct(raw) {
  const source = raw && typeof raw === 'object' ? raw : {}
  const sku = String(source.sku ?? '')
  const salePrice = decimal(pick(source.salePrice, source.sale_price))
  const pinnedPrice = decimal(pick(source.pinnedPrice, source.pinned_price))
  // A missing effective price is derived, never left to a sales surface to
  // guess: a pin wins over sale_price and 0 means unpriced.
  const effectivePrice = decimal(pick(source.effectivePrice, source.effective_price)) || pinnedPrice || salePrice
  const vatRate = source.vatRate ?? source.vat_rate ?? '20'
  const active = boolFlag(source.active)
  const priced = Number(effectivePrice) > 0
  const noSku = boolFlag(source.noSku ?? source.no_sku) || /^NO-SKU-/i.test(sku)
  const displaySku = String(source.displaySku ?? source.display_sku ?? (noSku ? 'NO SKU' : sku))
  const skuWarning = source.skuWarning ?? source.sku_warning ?? (noSku ? 'NO SKU — Urgent: update model code from carton' : null)
  const isReturn = boolFlag(source.isReturn ?? source.is_return)
  const returnQty = Number(source.returnQty ?? source.return_qty) || 0
  const stock = Number(source.stock) || 0
  const sealedQty = source.sealedQty !== undefined || source.sealed_qty !== undefined
    ? (Number(source.sealedQty ?? source.sealed_qty) || 0)
    : Math.max(0, stock - returnQty)
  const returnLabel = source.returnLabel ?? source.return_label ?? (isReturn ? (sealedQty === 0 ? 'Return' : `Return (${returnQty}/${stock} units)`) : null)
  const priceStatus = String(source.priceStatus ?? source.price_status ?? (priced ? 'PRICED' : 'NO_PRICE'))

  return {
    ...source,
    sku,
    displaySku,
    display_sku: displaySku,
    noSku,
    no_sku: noSku,
    skuWarning,
    sku_warning: skuWarning,
    isReturn,
    is_return: isReturn,
    returnQty,
    return_qty: returnQty,
    sealedQty,
    sealed_qty: sealedQty,
    returnLabel,
    return_label: returnLabel,
    priceStatus,
    price_status: priceStatus,
    priceLabel: priced ? `${effectivePrice} DH` : 'No price available',
    name: String(source.name ?? ''),
    salePrice,
    pinnedPrice,
    effectivePrice,
    priced,
    vatRate,
    stock,
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

  function loadCached() {
    if (typeof localStorage === 'undefined') return
    try {
      const raw = localStorage.getItem(CATALOG_CACHE_KEY)
      if (!raw) return
      const parsed = JSON.parse(raw)
      if (Array.isArray(parsed.products)) {
        products.value = parsed.products.map(normalizeProduct)
        version.value = parsed.version ?? 0
        snapshotAt.value = parsed.snapshotAt || ''
      }
    } catch { /* ignore corrupt cache */ }
  }

  function saveCached() {
    if (typeof localStorage === 'undefined') return
    try {
      localStorage.setItem(CATALOG_CACHE_KEY, JSON.stringify({
        version: version.value,
        snapshotAt: snapshotAt.value,
        products: products.value,
      }))
    } catch { /* quota full */ }
  }

  // Pricing surfaces need the full snapshot to count/report unpriced products,
  // then hide them locally. include_unpriced=1 also disables the 304 branch so
  // switching modes cannot serve a stale filtered response.
  async function refresh(includeUnpriced = true) {
    if (!products.value.length) loadCached()
    const flag = includeUnpriced ? '&include_unpriced=1' : ''
    const res = await apiFetch(`/api/catalog?version=${version.value}${flag}`)
    if (!includeUnpriced && res.status === 304) return
    if (!res.ok) throw new Error(`catalog refresh failed (${res.status})`)
    const data = await res.json()
    products.value = (data.products || []).map(normalizeProduct)
    version.value = data.version ?? version.value
    snapshotAt.value = data.snapshotAt || new Date().toISOString()
    saveCached()
  }

  return { products, version, snapshotAt, refresh, loadCached, normalizeProduct }
})
