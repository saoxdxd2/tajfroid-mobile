<template>
  <div class="view">
    <h1>{{ COPY.pairTitle }}</h1>
    <p class="muted">{{ COPY.pairIntro }}</p>
    <div v-if="route.query.reason === 'expired'" class="notice" role="status">{{ COPY.pairExpired }}</div>
    <div v-if="autoPairing" class="status">{{ COPY.pairAutoConnecting }}</div>
    <template v-else>
      <label for="pair-code">{{ COPY.pairCodeLabel }}</label>
      <input id="pair-code" v-model="code" :placeholder="COPY.pairCodeLabel" aria-describedby="pair-code-help" autocomplete="one-time-code" />
      <p id="pair-code-help" class="muted">{{ COPY.pairCodeHelp }}</p>
      <details class="advanced">
        <summary>{{ COPY.pairAdvanced }}</summary>
        <label for="pair-relay">{{ COPY.pairRelayLabel }}</label>
        <input id="pair-relay" v-model="relayUrl" :placeholder="COPY.pairRelayLabel" aria-describedby="pair-relay-help" />
        <p id="pair-relay-help" class="muted">{{ COPY.pairRelayHelp }}</p>
      </details>
      <button class="btn-primary" @click="pair">{{ COPY.connectToComputer }}</button>
    </template>
    <p v-if="status" class="status" role="status">{{ status }}</p>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useDeviceStore } from '../stores/device'
import { useRoute } from 'vue-router'
import { COPY, friendlyMessage } from '../lib/copy'

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
  status.value = COPY.pairAutoConnecting
  try {
    await store.pair(code.value)
    status.value = COPY.pairSuccess
  } catch (e) {
    status.value = friendlyMessage(e) || COPY.pairFailed
    autoPairing.value = false
  }
}

async function pair() {
  if (!code.value) {
    status.value = COPY.pairMissingCode
    return
  }
  if (!relayUrl.value) {
    status.value = COPY.pairMissingRelay
    return
  }
  store.setRelayUrl(relayUrl.value)
  status.value = COPY.pairConnecting
  try {
    await store.pair(code.value)
    status.value = COPY.pairSuccess
  } catch (e) {
    status.value = friendlyMessage(e) || COPY.pairFailed
  }
}
</script>

<style scoped>
.muted { color: #5b6b7c; font-size: 14px; }
.notice { border: 1px solid #f0c36a; background: #fff8e1; color: #6b4e00; border-radius: 8px; padding: 10px 12px; margin: 12px 0; }
.status { margin-top: 12px; }
label { display: block; margin-top: 12px; font-weight: 600; }
input { width: 100%; }
.advanced { margin: 12px 0; }
.advanced summary { cursor: pointer; padding: 8px 0; color: #14509b; }
</style>
