<template>
  <div>
    <div class="tabs">
      <button :class="{ active: tab === 'customers' }" @click="tab = 'customers'">Customers</button>
      <button :class="{ active: tab === 'suppliers' }" @click="tab = 'suppliers'">Suppliers</button>
    </div>
    <CustomersView v-show="tab === 'customers'" />
    <SuppliersView v-show="tab === 'suppliers'" />
  </div>
</template>

<script setup>
import { ref } from 'vue'
import CustomersView from './CustomersView.vue'
import SuppliersView from './SuppliersView.vue'

const tab = ref('customers')
</script>

<style scoped>
.tabs { display: flex; gap: 8px; padding: 0 0 8px; max-width: 600px; margin: 0 auto; }
.tabs button { flex: 1; padding: 10px; border-radius: 8px; }
.tabs button.active { background: #1f3a5f; color: #fff; border-color: #1f3a5f; }
</style>
