<template>
  <div class="app">
    <header class="app-header">
      <span class="brand">{{ COPY.brand }}</span>
      <router-link :to="paired ? '/account' : '/pair'" class="connection-pill" :class="{ connected: paired }">
        {{ paired ? COPY.connectedToLaptop : COPY.connectToComputer }}
      </router-link>
    </header>
    <div v-if="!online" class="offline-banner" role="status">{{ COPY.offlineBanner }}</div>
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
    </nav>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import MutationStatus from './components/MutationStatus.vue'
import { COPY } from './lib/copy'
import { useDeviceStore } from './stores/device'
import { useMutationStore } from './stores/mutations'
import { useQueueStore } from './stores/queue'
import { useInventoryStore } from './stores/inventory'

const mutations = useMutationStore()
const scans = useQueueStore()
const inventory = useInventoryStore()
const device = useDeviceStore()
const paired = computed(() => !!device.deviceToken)
const online = ref(typeof navigator === 'undefined' ? true : navigator.onLine)
const unpricedCount = computed(() => inventory.products.filter(p => !p.priced).length)

function updateOnline() {
  online.value = typeof navigator === 'undefined' ? true : navigator.onLine
}

onMounted(async () => {
  window.addEventListener('online', updateOnline)
  window.addEventListener('offline', updateOnline)
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
.app-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px calc(8px + env(safe-area-inset-top));
  background: #1f3a5f;
  color: #fff;
}
.brand { font-weight: 700; letter-spacing: 0.04em; }
.offline-banner {
  padding: 8px 12px;
  background: #fff3cd;
  color: #6b4e00;
  border-bottom: 1px solid #f0c36a;
  font-size: 14px;
}
main { padding-bottom: 150px; }
.badge { display: inline-block; padding: 1px 6px; border-radius: 10px; background: #c62828; color: #fff; font-size: 0.75em; }
</style>
