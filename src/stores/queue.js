import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useQueueStore = defineStore('queue', () => {
  const items = ref([])

  async function enqueue(file) {
    // Downscale to 2000px / 2.5MB / JPEG 0.82 (placeholder: store original).
    const clientId = crypto.randomUUID()
    items.value.push({ clientId, status: 'En attente', purchaseReference: null, message: null })
    // TODO: encrypt with libsodium sealed box to desktop public key, then upload.
  }

  async function flush(relayUrl, deviceToken) {
    // Upload pending items; update statuses.
  }

  return { items, enqueue, flush }
})