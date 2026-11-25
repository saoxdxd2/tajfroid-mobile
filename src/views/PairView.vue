<template>
  <div class="bg-white rounded-3xl border border-slate-200 p-5 shadow-sm space-y-4">
    <div>
      <h1 class="text-xl font-bold text-slate-900 m-0">{{ COPY.pairTitle }}</h1>
      <p class="text-xs text-slate-500 mt-1">{{ COPY.pairIntro }}</p>
    </div>

    <div v-if="route.query.reason === 'expired'" class="p-3 rounded-xl bg-amber-50 border border-amber-200 text-xs text-amber-900 font-medium" role="status">
      {{ COPY.pairExpired }}
    </div>

    <div v-if="autoPairing" class="p-4 rounded-2xl bg-blue-50 text-blue-900 text-sm font-semibold text-center">
      {{ COPY.pairAutoConnecting }}
    </div>

    <template v-else>
      <label class="block text-xs font-semibold text-slate-700">
        {{ COPY.pairCodeLabel }}
        <input
          id="pair-code"
          v-model="code"
          :placeholder="COPY.pairCodeLabel"
          autocomplete="one-time-code"
          class="mt-1 w-full min-h-[48px] px-3.5 rounded-xl border border-slate-200 bg-slate-50 font-mono text-base tracking-widest uppercase"
        />
      </label>
      <p class="text-xs text-slate-500 m-0">{{ COPY.pairCodeHelp }}</p>

      <details class="text-xs text-slate-600">
        <summary class="cursor-pointer font-semibold text-blue-700 py-1">{{ COPY.pairAdvanced }}</summary>
        <label class="block mt-2 font-semibold text-slate-700">
          {{ COPY.pairRelayLabel }}
          <input id="pair-relay" v-model="relayUrl" :placeholder="COPY.pairRelayLabel" class="mt-1 w-full min-h-[44px] px-3 rounded-xl border border-slate-200 bg-slate-50 text-xs" />
        </label>
      </details>

      <button type="button" @click="pair" class="w-full min-h-[48px] rounded-xl bg-blue-700 text-white font-semibold text-sm shadow-xs">
        {{ COPY.connectToComputer }}
      </button>
    </template>

    <p v-if="status" class="text-xs font-semibold text-slate-700" role="status">{{ status }}</p>
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
  const q = route.query
  if (q.relay && q.code) {
    relayUrl.value = q.relay
    code.value = q.code
    if (q.pk) store.setDesktopPublicKey(q.pk)
    store.setRelayUrl(q.relay)
    autoPair()
  } else {
    relayUrl.value = store.relayUrl || (typeof location !== 'undefined' ? location.origin : '')
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
  if (!code.value) { status.value = COPY.pairMissingCode; return }
  if (!relayUrl.value) { status.value = COPY.pairMissingRelay; return }
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
