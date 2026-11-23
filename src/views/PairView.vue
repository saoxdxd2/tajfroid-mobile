<template>
  <div class="view">
    <h1>Pair with desktop</h1>
    <p>Scan the QR code shown on the desktop ERP, or enter the pairing code.</p>
    <div v-if="autoPairing" class="status">Pairing automatically...</div>
    <template v-else>
      <input v-model="code" placeholder="Pairing code" />
      <input v-model="relayUrl" placeholder="Relay URL" />
      <button @click="pair">Pair</button>
    </template>
    <p v-if="status">{{ status }}</p>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useDeviceStore } from '../stores/device'
import { useRoute } from 'vue-router'

const code = ref('')
const relayUrl = ref('')
const status = ref('')
const autoPairing = ref(false)
const store = useDeviceStore()
const route = useRoute()

onMounted(() => {
  // If opened via QR scan, params are in the URL query string
  const q = route.query
  if (q.relay && q.code) {
    relayUrl.value = q.relay
    code.value = q.code
    if (q.pk) store.setDesktopPublicKey(q.pk)
    store.setRelayUrl(q.relay)
    autoPair()
  } else {
    relayUrl.value = store.relayUrl || ''
  }
})

async function autoPair() {
  autoPairing.value = true
  status.value = 'Pairing automatically...'
  try {
    await store.pair(code.value)
    status.value = 'Paired successfully!'
  } catch (e) {
    status.value = 'Auto-pair failed: ' + e.message
    autoPairing.value = false
  }
}

async function pair() {
  if (!relayUrl.value) {
    status.value = 'Enter a relay URL.'
    return
  }
  store.setRelayUrl(relayUrl.value)
  status.value = 'Pairing...'
  try {
    await store.pair(code.value)
    status.value = 'Paired successfully!'
  } catch (e) {
    status.value = 'Failed: ' + e.message
  }
}
</script>