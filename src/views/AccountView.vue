<template>
  <div class="view">
    <h1>Account</h1>
    <p>Device paired: {{ paired ? 'yes' : 'no' }}</p>
    <p class="muted">Relay: {{ relayUrl || 'not set' }}</p>

    <h2>Paired devices</h2>
    <button @click="loadDevices" :disabled="loading">{{ loading ? 'Loading…' : 'Refresh' }}</button>
    <p v-if="error" class="err">{{ error }}</p>
    <ul class="list">
      <li v-for="d in devices" :key="d.id" class="card">
        <div class="row">
          <strong>{{ d.name || 'Device' }}</strong>
          <span :class="d.revokedAt ? 'revoked' : 'active'">{{ d.revokedAt ? 'revoked' : 'active' }}</span>
        </div>
        <div class="meta">ID: {{ d.id.slice(0,12) }}… · paired {{ fmt(d.pairedAt) }}</div>
        <button v-if="!d.revokedAt" class="danger" @click="revoke(d.id)">Revoke</button>
      </li>
    </ul>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useDeviceStore } from '../stores/device'
import { apiFetch, readError } from '../lib/api'

const store = useDeviceStore()
const paired = computed(() => !!store.deviceToken)
const relayUrl = computed(() => store.relayUrl)
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
  try {
    const res = await apiFetch(`/api/devices/manage?id=${encodeURIComponent(id)}`, { method: 'DELETE' })
    if (!res.ok) throw new Error(await readError(res, 'Failed to revoke'))
    await loadDevices()
  } catch (e) { error.value = e.message }
}

function fmt(ts) { if (!ts) return ''; return new Date(ts * 1000).toLocaleString() }
onMounted(loadDevices)
</script>

<style scoped>
.muted { color:#666; font-size:13px; }
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
