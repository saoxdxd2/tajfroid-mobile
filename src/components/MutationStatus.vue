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
        <details v-if="m.body" class="payload"><summary>Payload / merge</summary>
          <div v-if="isFlat(m.body)" class="fields">
            <label v-for="(v,k) in m.body" :key="k" class="field">
              <span>{{ k }}</span>
              <input :value="fieldValue(m.id,k,v)" @input="setField(m.id,k,$event.target.value,v)" />
            </label>
          </div>
          <textarea v-else v-model="edits[m.id]" rows="5" class="edit"></textarea>
          <div class="actions"><button @click="savePayload(m.id)">Save payload</button></div>
        </details>
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
import { computed, reactive, ref, watch } from 'vue'
import { useMutationStore } from '../stores/mutations'
const store = useMutationStore()
const open = ref(false)
const edits = reactive({})
const items = computed(() => store.items)
const syncing = computed(() => store.syncing)
const pending = computed(() => items.value.filter(i => i.status === 'pending').length)
const conflicts = computed(() => items.value.filter(i => i.status === 'conflict').length)
watch(items, (list) => { for (const m of list) if (edits[m.id] === undefined) edits[m.id] = pretty(m.body) }, { immediate: true, deep: true })
function retry(id) { store.retry(id) }
function remove(id) { store.remove(id) }
function dismiss(id) { store.remove(id) }
function pretty(v) { try { return JSON.stringify(v ?? {}, null, 2) } catch { return String(v) } }
function isFlat(v) { return v && typeof v === 'object' && !Array.isArray(v) && Object.values(v).every(x => ['string','number','boolean'].includes(typeof x) || x === null) }
function fieldValue(id,k,orig) { try { const o = JSON.parse(edits[id]||'{}'); return o[k] ?? '' } catch { return orig ?? '' } }
function setField(id,k,val,orig) {
  let o = {}
  try { o = JSON.parse(edits[id] || '{}') } catch { o = {} }
  if (typeof orig === 'number') { const n = Number(val); o[k] = Number.isFinite(n) ? n : val }
  else if (typeof orig === 'boolean') { o[k] = val === 'true' }
  else o[k] = val
  edits[id] = pretty(o)
}
async function savePayload(id) {
  try {
    const body = JSON.parse(edits[id] || '{}')
    await store.updateBody(id, body)
  } catch (e) { alert('Invalid JSON: ' + e.message) }
}
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
.edit { width: 100%; font-family: monospace; font-size: 11px; margin: 4px 0; }
.fields { display: grid; gap: 4px; margin: 4px 0; }
.field { display: grid; grid-template-columns: minmax(80px, 1fr) 2fr; gap: 6px; align-items: center; }
.field input { width: 100%; font-size: 11px; padding: 2px 4px; }
.actions { display: flex; gap: 8px; flex-wrap: wrap; }
.actions button { font-size: 11px; padding: 2px 8px; }
.hint { color: #555; margin: 8px 0 0; }
</style>
