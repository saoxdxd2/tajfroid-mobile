<template>
  <div class="view">
    <div class="header">
      <h1>{{ COPY.queueTitle }}</h1>
      <button @click="sync" :disabled="!canSync || queue.syncing">{{ syncLabel }}</button>
    </div>
    <p class="hint">{{ canSync ? COPY.queueHint : COPY.queueNotConnected }}</p>
    <p v-if="!canSync" class="hint"><router-link to="/pair">{{ COPY.connectToComputer }}</router-link></p>
    <p v-if="queue.lastSyncError" class="err">{{ lastError }}</p>
    <p v-if="!items.length" class="empty-state">{{ COPY.queueEmpty }}</p>
    <ul class="list">
      <li v-for="s in items" :key="s.clientId" class="card">
        <div class="row">
          <strong class="status" :data-status="s.status">{{ label(s.status) }}</strong>
          <small>{{ fmtTime(s.capturedAt) }}</small>
        </div>
        <div class="meta">ID: {{ s.clientId.slice(0, 8) }}… <span v-if="s.sizeBytes">· {{ Math.round(s.sizeBytes/1024) }} KB</span></div>
        <div v-if="s.purchaseReference" class="ok">Purchase: {{ s.purchaseReference }}</div>
        <div v-if="s.message" class="msg">{{ s.message }}</div>
        <div class="actions">
          <button v-if="canRetry(s.status)" @click="queue.flush()">Retry upload</button>
          <button v-if="isTerminal(s.status) || s.status==='failed_local'" class="danger" @click="remove(s.clientId)">Remove</button>
        </div>
      </li>
    </ul>
  </div>
</template>

<script setup>
import { computed, onMounted } from 'vue'
import { COPY, friendlyMessage } from '../lib/copy'
import { useDeviceStore } from '../stores/device'
import { useQueueStore } from '../stores/queue'
const device = useDeviceStore()
const queue = useQueueStore()
const items = computed(() => queue.items)
const canSync = computed(() => !!(device.relayUrl && device.deviceToken))
const syncLabel = computed(() => queue.syncing ? COPY.queueSending : canSync.value ? COPY.queueSyncNow : COPY.queuePairToSync)
const lastError = computed(() => queue.lastSyncError ? friendlyMessage(queue.lastSyncError) : '')

onMounted(async () => {
  await queue.load()
  queue.flush().catch(() => {})
})

function sync() { queue.flush().catch(() => {}) }
function remove(id) { queue.remove(id) }
function isTerminal(s) { return ['imported', 'rejected', 'failed'].includes(s) }
function canRetry(s) { return ['upload_failed', 'pending_upload'].includes(s) }
function fmtTime(ts) { if (!ts) return ''; return new Date(ts * 1000).toLocaleString() }
function label(s) { return COPY.queueStatus[s] || s }
</script>

<style scoped>
.header { display:flex; align-items:center; justify-content:space-between; gap:12px; }
.hint { color:#5b6b7c; font-size:14px; margin:4px 0 12px; }
.err { color:#b00020; font-size:14px; }
.list { list-style:none; padding:0; margin:0; display:flex; flex-direction:column; gap:10px; }
.card { border:1px solid #c3cedb; border-radius:10px; padding:12px; background:#fff; }
.row { display:flex; justify-content:space-between; gap:8px; }
.status { font-size:14px; }
.status[data-status="imported"] { color:#1b7f3a; }
.status[data-status="failed"], .status[data-status="failed_local"], .status[data-status="rejected"], .status[data-status="upload_failed"] { color:#b00020; }
.meta, .msg { color:#5b6b7c; font-size:14px; margin-top:4px; }
.ok { color:#1b7f3a; font-size:14px; margin-top:4px; }
.actions { display:flex; gap:8px; margin-top:8px; }
button { padding:8px 12px; }
button.danger { color:#b00020; }
</style>
