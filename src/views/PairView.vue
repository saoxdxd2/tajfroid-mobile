<template>
  <div class="view">
    <h1>Pair with desktop</h1>
    <p>Scan the QR code shown on the desktop ERP, or enter the pairing code.</p>
    <input v-model="code" placeholder="Pairing code" />
    <button @click="pair">Pair</button>
    <p v-if="status">{{ status }}</p>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useDeviceStore } from '../stores/device'
const code = ref('')
const status = ref('')
const store = useDeviceStore()
async function pair() {
  status.value = 'Pairing...'
  try {
    await store.pair(code.value)
    status.value = 'Paired.'
  } catch (e) {
    status.value = 'Failed: ' + e.message
  }
}
</script>