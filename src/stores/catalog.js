import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiFetch } from '../lib/api'

export const useCatalogStore = defineStore('catalog', () => {
  const products = ref([])
  const version = ref(0)

  async function refresh() {
    const res = await apiFetch(`/api/catalog?version=${version.value}`)
    if (res.status === 304) return
    if (!res.ok) throw new Error(`catalog refresh failed (${res.status})`)
    const data = await res.json()
    products.value = data.products
    version.value = data.version
  }

  return { products, version, refresh }
})
