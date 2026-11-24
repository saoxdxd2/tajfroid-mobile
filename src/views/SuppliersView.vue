<template>
  <div class="view">
    <h1>Suppliers</h1>
    <div class="toolbar">
      <input v-model="q" placeholder="Search name / phone / ICE" />
      <button @click="showAdd = !showAdd">{{ showAdd ? 'Cancel' : '+ Add' }}</button>
      <button @click="load">Refresh</button>
    </div>

    <div v-if="showAdd" class="form-card">
      <h3>New supplier</h3>
      <select v-model="form.partyType">
        <option value="enterprise">Enterprise</option>
        <option value="individual">Individual</option>
      </select>
      <template v-if="form.partyType === 'individual'">
        <input v-model="form.firstName" placeholder="First name" />
        <input v-model="form.lastName" placeholder="Last name" />
      </template>
      <template v-else>
        <input v-model="form.companyName" placeholder="Company name" />
      </template>
      <input v-model="form.contactName" placeholder="Contact name" />
      <input v-model="form.phone" placeholder="Phone" />
      <input v-model="form.email" placeholder="Email" />
      <input v-model="form.iceNumber" placeholder="ICE number" />
      <button @click="addSupplier">Save</button>
      <p v-if="msg" class="msg">{{ msg }}</p>
    </div>

    <p v-if="store.loading">Loading...</p>
    <p v-if="store.error" class="error">{{ store.error }}</p>

    <ul class="item-list">
      <li v-for="s in filtered" :key="s.id" class="item">
        <div class="item-header">
          <strong>{{ s.companyName }}</strong>
          <span class="badge">{{ s.partyType }}</span>
        </div>
        <div class="item-details">
          <span v-if="s.phone">📞 {{ s.phone }}</span>
          <span v-if="s.email">✉ {{ s.email }}</span>
          <span v-if="s.iceNumber">ICE: {{ s.iceNumber }}</span>
          <span v-if="s.contactName">Contact: {{ s.contactName }}</span>
        </div>
        <div class="item-actions">
          <button @click="startEdit(s)">Edit</button>
          <button @click="remove(s)" class="danger">Archive</button>
        </div>
      </li>
    </ul>
    <p v-if="!store.loading && !filtered.length" class="empty-state">{{ COPY.empty.suppliers }}</p>

    <div v-if="editing" class="modal-overlay" @click.self="editing = null">
      <div class="modal">
        <h3>Edit supplier</h3>
        <input v-model="editForm.companyName" placeholder="Company name" />
        <input v-model="editForm.contactName" placeholder="Contact name" />
        <input v-model="editForm.phone" placeholder="Phone" />
        <input v-model="editForm.email" placeholder="Email" />
        <input v-model="editForm.iceNumber" placeholder="ICE number" />
        <div class="modal-actions">
          <button @click="saveEdit">Save</button>
          <button @click="editing = null">Cancel</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useSupplierStore } from '../stores/suppliers'
import { COPY } from '../lib/copy'

const store = useSupplierStore()
const q = ref('')
const showAdd = ref(false)
const msg = ref('')
const editing = ref(null)

const form = ref({
  partyType: 'enterprise', companyName: '', firstName: '', lastName: '',
  contactName: '', phone: '', email: '', iceNumber: '',
})

const editForm = ref({})

const filtered = computed(() => {
  const term = q.value.toLowerCase()
  if (!term) return store.suppliers
  return store.suppliers.filter(s =>
    (s.companyName + ' ' + (s.phone || '') + ' ' + (s.iceNumber || '') + ' ' + (s.contactName || '')).toLowerCase().includes(term))
})

onMounted(() => store.fetchAll())

async function load() { await store.fetchAll() }

async function addSupplier() {
  try {
    await store.create(form.value)
    msg.value = 'Supplier created!'
    form.value = { partyType: 'enterprise', companyName: '', firstName: '', lastName: '', contactName: '', phone: '', email: '', iceNumber: '' }
    showAdd.value = false
  } catch (e) {
    msg.value = 'Error: ' + e.message
  }
}

function startEdit(s) {
  editing.value = s
  editForm.value = {
    companyName: s.companyName, contactName: s.contactName || '',
    phone: s.phone || '', email: s.email || '', iceNumber: s.iceNumber || '',
  }
}

async function saveEdit() {
  try {
    await store.update(editing.value.id, { ...editForm.value, baseUpdatedAt: editing.value.updatedAt })
    editing.value = null
  } catch (e) {
    alert('Error: ' + e.message)
  }
}

async function remove(s) {
  if (!confirm('Archive this supplier?')) return
  try { await store.archive(s.id, s.updatedAt) } catch (e) { alert('Error: ' + e.message) }
}
</script>

<style scoped>
.toolbar { display: flex; gap: 8px; margin-bottom: 12px; flex-wrap: wrap; }
.toolbar input { flex: 1; min-width: 150px; padding: 8px; }
.form-card { background: #f5f5f5; padding: 12px; border-radius: 8px; margin-bottom: 12px; }
.form-card input, .form-card select { display: block; width: 100%; margin: 4px 0; padding: 8px; }
.item-list { list-style: none; padding: 0; }
.item { border: 1px solid #ddd; border-radius: 8px; padding: 12px; margin-bottom: 8px; }
.item-header { display: flex; justify-content: space-between; align-items: center; }
.item-details { display: flex; gap: 12px; flex-wrap: wrap; font-size: 0.9em; color: #666; margin: 4px 0; }
.item-actions { display: flex; gap: 6px; margin-top: 8px; }
.badge { padding: 2px 8px; border-radius: 12px; font-size: 0.8em; background: #fce4ec; color: #c62828; }
.msg { color: green; }
.error { color: red; }
.modal-overlay { position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.5); display: flex; align-items: center; justify-content: center; z-index: 200; }
.modal { background: #fff; padding: 20px; border-radius: 8px; width: 90%; max-width: 400px; }
.modal input { display: block; width: 100%; margin: 4px 0; padding: 8px; }
.modal-actions { display: flex; gap: 8px; margin-top: 12px; }
.danger { color: red; }
</style>