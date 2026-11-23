<template>
  <div v-if="items.length || syncing" class="mutation-status">
    <div class="row">
      <strong>Offline queue</strong>
      <span v-if="syncing">syncing…</span>
      <span v-else>{{ pending }} pending, {{ conflicts }} conflicts</span>
      <button class="link" @click="open = !open">{{ open ? 'Hide' : 'Details' }}</button>
    </div>
    <div v-if="open" class="details">
      <div v-for="m in items" :key="m.id" class="item" :class="m.status">
        <div class="meta">
          <span class="badge">{{ m.status }}</span>
          <span>{{ m.entity || 'mutation' }} {{ m.method }}</span>
        </div>
        <div class="path">{{ m.path }}</div>
        <div v-if="m.lastError" class="err">{{ m.lastError }}</div>
        <details v-if="m.body" class="payload"><summary>Payload</summary><pre>{{ pretty(m.body) }}</pre></details>
        <div class="actions">
          <button v-if="m.status==='conflict'" @click="retry(m.id)">Retry local</button>
          <button v-if="m.status==='conflict'" @click="dismiss(m.id)">Keep server (dismiss)</button>
          <button v-if="m.status!=='conflict'" @click="retry(m.id)">Retry</button>
          <button v-if="m.status!=='conflict'" @click="remove(m.id)">Dismiss</button>
        </div>
      </div>
      <p class="hint">Conflicts mean another device changed the same record. "Retry local" resends your payload; "Keep server" discards your queued change.</p>
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'
import { useMutationStore } from '../stores/mutations'
const store = useMutationStore()
const open = ref(false)
const items = computed(() => store.items)
const syncing = computed(() => store.syncing)
const pending = computed(() => items.value.filter(i => i.status === 'pending').length)
const conflicts = computed(() => items.value.filter(i => i.status === 'conflict').length)
function retry(id) { store.retry(id) }
function remove(id) { store.remove(id) }
function dismiss(id) { store.remove(id) }
function pretty(v) { try { return JSON.stringify(v, null, 2) } catch { return String(v) } }
</script>

<style scoped>
.mutation-status { position: fixed; left: 8px; right: 8px; bottom: 76px; background: #fff8e1; border: 1px solid #f0c36a; border-radius: 8px; padding: 8px; z-index: 50; font-size: 12px; }
.row { display: flex; gap: 8px; align-items: center; }
.link { margin-left: auto; background: none; border: none; color: #1f3a5f; text-decoration: underline; }
.details { margin-top: 8px; max-height: 35vh; overflow: auto; }
.item { border-top: 1px solid #ecd9a3; padding: 6px 0; }
.item.conflict .badge { background: #b71c1c; }
.meta { display: flex; gap: 8px; align-items: center; }
.badge { background: #8a6d00; color: #fff; padding: 1px 6px; border-radius: 10px; font-size: 11px; }
.path { color: #555; word-break: break-all; }
.err { color: #7a1a00; margin: 4px 0; white-space: pre-wrap; }
.payload pre { max-height: 120px; overflow: auto; background: #fff; padding: 4px; margin: 4px 0; font-size: 11px; }
.actions { display: flex; gap: 8px; flex-wrap: wrap; }
.actions button { font-size: 11px; padding: 2px 8px; }
.hint { color: #555; margin: 8px 0 0; }
</style>
