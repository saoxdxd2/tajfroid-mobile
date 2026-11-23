import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useCatalogStore = defineStore('catalog', () => {
  const products = ref([])
  const version = ref(0)
  const snapshotAt = ref('')

  async function refresh(relayUrl, deviceToken) {
    const res = await fetch(`${relayUrl}/api/catalog?version=${version.value}`, {
      headers: { Authorization: `Bearer ${deviceToken}` },
    })
    if (res.status === 304) return
    if (!res.ok) throw new Error('catalog fetch failed')
    const data = await res.json()
    products.value = data.products || []
    version.value = data.version
    snapshotAt.value = new Date().toISOString()
    // Persist to IndexedDB could go here.
  }

  return { products, version, snapshotAt, refresh }
})