<template>
  <div class="view">
    <h1>Account</h1>
    <div class="connection-card" :class="{ connected: paired }">
      <strong>{{ paired ? COPY.connectedToLaptop : COPY.connectToComputer }}</strong>
      <p>{{ paired ? COPY.accountConnected : COPY.accountNotConnected }}</p>
      <p class="muted">{{ COPY.accountHint }}</p>
      <router-link to="/pair" class="btn-primary">
        {{ paired ? 'Reconnect or manage connection' : 'Connect to computer' }}
      </router-link>
    </div>

    <h2>Connected devices</h2>
    <button @click="loadDevices" :disabled="loading">{{ loading ? 'Loading…' : 'Refresh' }}</button>
    <p v-if="error" class="err">{{ error }}</p>
    <ul class="list">
      <li v-for="d in devices" :key="d.id" class="card">
        <div class="row">
          <strong>{{ d.name || 'Device' }} <span v-if="d.id===currentDeviceId" class="me">(this device)</span></strong>
          <span :class="d.revokedAt ? 'revoked' : 'active'">{{ d.revokedAt ? 'revoked' : 'active' }}</span>
        </div>
        <div class="meta">ID: {{ d.id.slice(0,12) }}… · paired {{ fmt(d.pairedAt) }}</div>
        <button v-if="!d.revokedAt && d.id!==currentDeviceId" class="danger" @click="revoke(d.id)">Revoke</button>
        <span v-if="!d.revokedAt && d.id===currentDeviceId" class="muted">Current device cannot be revoked here.</span>
      </li>
    </ul>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useDeviceStore } from '../stores/device'
import { useMutationStore } from '../stores/mutations'
import { apiFetch, jsonHeaders, readError } from '../lib/api'
import { COPY } from '../lib/copy'

const store = useDeviceStore()
const mutations = useMutationStore()
const paired = computed(() => !!store.deviceToken)
const currentDeviceId = computed(() => store.deviceId)
const devices = ref([])
const loading = ref(false)
const error = ref('')

async function loadDevices() {
  if (!store.deviceToken) return
  loading.value = true; error.value = ''
  try {
    const res = await apiFetch('/api/devices')
    if (!res.ok) throw new Error(await readError(res, 'Failed to load devices'))
    const data = await res.json()
    devices.value = data.devices || []
  } catch (e) { error.value = e.message }
  finally { loading.value = false }
}

async function revoke(id) {
  if (!confirm('Revoke this device? It will need to pair again.')) return
  const path = `/api/devices/manage?id=${encodeURIComponent(id)}`
  const found = devices.value.find(d => d.id === id)
  const base = Number(found?.baseRevokedAt || found?.revokedAt || 0)
  const body = { baseRevokedAt: base }
  try {
    const res = await apiFetch(path, {
      method: 'DELETE', headers: jsonHeaders(), body: JSON.stringify(body),
    })
    if (res.status === 409) {
      await mutations.conflictFromResponse(res, {
        method: 'DELETE', path, body, entity: 'device', entityId: id,
        baseUpdatedAt: base, fallback: 'Conflict: device changed',
      })
    }
    if (!res.ok) throw new Error(await readError(res, 'Failed to revoke'))
    await loadDevices()
  } catch (e) {
    if (!e.conflict && (!navigator.onLine || e instanceof TypeError)) {
      await mutations.enqueue({
        method: 'DELETE', path, body, entity: 'device', entityId: id, baseUpdatedAt: base,
      })
      error.value = 'Offline: revoke queued'
      return
    }
    error.value = e.message
  }
}

function fmt(ts) { if (!ts) return ''; return new Date(ts * 1000).toLocaleString() }
onMounted(loadDevices)
</script>

<style scoped>
.muted { color:#666; font-size:13px; }
.me { color:#1565c0; font-size:12px; font-weight:normal; }
.err { color:#b00020; }
.list { list-style:none; padding:0; margin:12px 0; display:flex; flex-direction:column; gap:10px; }
.card { border:1px solid #ddd; border-radius:10px; padding:12px; background:#fff; }
.row { display:flex; justify-content:space-between; gap:8px; }
.meta { color:#555; font-size:13px; margin:4px 0 8px; }
.active { color:#1b7f3a; font-size:13px; }
.revoked { color:#b00020; font-size:13px; }
button { padding:8px 12px; }
button.danger { color:#b00020; }
</style>
