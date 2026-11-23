<template>
  <div class="app">
    <main>
      <router-view />
    </main>
    <nav class="bottom-nav">
      <router-link to="/catalog">Catalog</router-link>
      <router-link to="/inventory">Inventory</router-link>
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
import { onMounted } from 'vue'
import { useMutationStore } from './stores/mutations'
import { useQueueStore } from './stores/queue'

const mutations = useMutationStore()
const scans = useQueueStore()

onMounted(async () => {
  await Promise.all([mutations.load(), scans.load()])
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
</style>
