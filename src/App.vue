<template>
  <div class="app">
    <main>
      <router-view />
    </main>
    <MutationStatus />
    <nav class="bottom-nav">
      <router-link to="/catalog">Catalog</router-link>
      <router-link to="/inventory">Inventory</router-link>
      <router-link to="/needs-pricing">
        Needs pricing <span v-if="unpricedCount" class="badge">{{ unpricedCount }}</span>
      </router-link>
      <router-link to="/sales">Sales</router-link>
      <router-link to="/purchases">Purchases</router-link>
      <router-link to="/contacts">Contacts</router-link>
      <router-link to="/finance">Finance</router-link>
      <router-link to="/scans">Scans</router-link>
      <router-link to="/account">Account</router-link>
    </nav>
  </div>
</template>

<script setup>
import { computed, onMounted } from 'vue'
import MutationStatus from './components/MutationStatus.vue'
import { useMutationStore } from './stores/mutations'
import { useQueueStore } from './stores/queue'
import { useInventoryStore } from './stores/inventory'

const mutations = useMutationStore()
const scans = useQueueStore()
const inventory = useInventoryStore()
const unpricedCount = computed(() => inventory.products.filter(p => !p.priced).length)

onMounted(async () => {
  await Promise.all([mutations.load(), scans.load(), inventory.fetchProducts()])
  mutations.flush().catch(() => {})
  scans.flush().catch(() => {})
  window.addEventListener('online', () => {
    mutations.flush().catch(() => {})
    scans.flush().catch(() => {})
  })
})
</script>

<style scoped>
main { padding-bottom: 150px; }
.badge { display: inline-block; padding: 1px 6px; border-radius: 10px; background: #c62828; color: #fff; font-size: 0.75em; }
</style>
