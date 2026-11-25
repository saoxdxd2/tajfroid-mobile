<template>
  <div class="flex flex-col h-full w-full bg-slate-100 text-slate-900">
    <!-- Top Header -->
    <header class="safe-top sticky top-0 z-30 bg-[#0f2544] text-white px-4 pb-3 shadow-sm">
      <div class="max-w-xl mx-auto flex items-center justify-between gap-3">
        <div class="flex items-center gap-2.5">
          <div class="w-8 h-8 rounded-lg bg-blue-500/20 border border-blue-400/30 flex items-center justify-center font-extrabold text-sm tracking-tight text-blue-200">
            TF
          </div>
          <div>
            <div class="font-bold text-sm tracking-wide leading-tight">{{ COPY.brand }}</div>
            <div class="text-[11px] text-slate-300 leading-tight">
              {{ online ? (mutations.syncing ? 'Syncing…' : 'Online · Local-First') : 'Offline · Queued' }}
            </div>
          </div>
        </div>
        <router-link
          :to="paired ? '/account' : '/pair'"
          class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs font-semibold transition active:scale-95 min-h-[36px]"
          :class="paired ? 'bg-emerald-500/20 text-emerald-200 border border-emerald-400/30' : 'bg-white/15 text-white border border-white/25'"
        >
          <span class="w-2 h-2 rounded-full" :class="paired ? 'bg-emerald-400' : 'bg-amber-300'"></span>
          <span>{{ paired ? 'Connected' : 'Pair Device' }}</span>
        </router-link>
      </div>
    </header>

    <!-- Offline Hold Banner -->
    <div v-if="!online" class="bg-amber-50 border-b border-amber-200 px-4 py-2 text-xs font-medium text-amber-900 text-center" role="status">
      Offline mode: edits are saved locally and scheduled to repush automatically.
    </div>

    <!-- Urgent NO SKU Alert Banner -->
    <router-link
      v-if="noSkuCount > 0"
      to="/inventory?filter=nosku"
      class="bg-rose-600 text-white px-4 py-2 text-xs font-semibold flex items-center justify-between gap-2 hover:bg-rose-700 transition"
    >
      <span class="flex items-center gap-2">
        <span class="inline-flex items-center justify-center w-5 h-5 rounded-full bg-white/20 font-bold">!</span>
        <span>Urgent: {{ noSkuCount }} product(s) have NO SKU — update from carton</span>
      </span>
      <span class="underline shrink-0">Fix now →</span>
    </router-link>

    <!-- Main Scroll Area -->
    <main class="flex-1 overflow-y-auto px-3.5 pt-3 pb-28">
      <div class="max-w-xl mx-auto">
        <router-view />
      </div>
    </main>

    <!-- Floating Sync / Conflict Queue Bar -->
    <MutationStatus />

    <!-- More Drawer Sheet -->
    <div v-if="moreOpen" class="fixed inset-0 z-50 bg-slate-900/50 backdrop-blur-xs flex items-end" @click.self="moreOpen = false">
      <div class="w-full max-w-xl mx-auto bg-white rounded-t-3xl p-5 pb-8 shadow-2xl space-y-4">
        <div class="flex items-center justify-between">
          <h2 class="text-base font-bold text-slate-900">Workspace Tools</h2>
          <button type="button" @click="moreOpen = false" class="px-3 py-1.5 rounded-lg bg-slate-100 text-slate-700 text-xs font-semibold">Close</button>
        </div>
        <div class="grid grid-cols-3 gap-2.5">
          <router-link to="/purchases" @click="moreOpen = false" class="flex flex-col items-center justify-center p-3 rounded-2xl bg-slate-50 border border-slate-200 text-slate-800 text-xs font-semibold gap-1.5 min-h-[72px]">
            <span>Purchases</span>
          </router-link>
          <router-link to="/contacts" @click="moreOpen = false" class="flex flex-col items-center justify-center p-3 rounded-2xl bg-slate-50 border border-slate-200 text-slate-800 text-xs font-semibold gap-1.5 min-h-[72px]">
            <span>Contacts</span>
          </router-link>
          <router-link to="/finance" @click="moreOpen = false" class="flex flex-col items-center justify-center p-3 rounded-2xl bg-slate-50 border border-slate-200 text-slate-800 text-xs font-semibold gap-1.5 min-h-[72px]">
            <span>Finance</span>
          </router-link>
          <router-link to="/scans" @click="moreOpen = false" class="flex flex-col items-center justify-center p-3 rounded-2xl bg-slate-50 border border-slate-200 text-slate-800 text-xs font-semibold gap-1.5 min-h-[72px]">
            <span>Scans</span>
          </router-link>
          <router-link to="/queue" @click="moreOpen = false" class="flex flex-col items-center justify-center p-3 rounded-2xl bg-slate-50 border border-slate-200 text-slate-800 text-xs font-semibold gap-1.5 min-h-[72px]">
            <span>Photo Queue</span>
          </router-link>
          <router-link :to="paired ? '/account' : '/pair'" @click="moreOpen = false" class="flex flex-col items-center justify-center p-3 rounded-2xl bg-slate-50 border border-slate-200 text-slate-800 text-xs font-semibold gap-1.5 min-h-[72px]">
            <span>{{ paired ? 'Devices' : 'Pair' }}</span>
          </router-link>
        </div>
      </div>
    </div>

    <!-- Clean 5-Slot Mobile Bottom Nav -->
    <nav class="safe-bottom fixed bottom-0 left-0 right-0 z-40 bg-white/95 backdrop-blur border-t border-slate-200 shadow-[0_-4px_20px_rgba(15,23,42,0.06)]">
      <div class="max-w-xl mx-auto grid grid-cols-5 h-16 px-1">
        <router-link to="/catalog" class="nav-tab" active-class="nav-tab-active">
          <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M4 6h16M4 12h16M4 18h10" /></svg>
          <span>Catalog</span>
        </router-link>
        <router-link to="/inventory" class="nav-tab relative" active-class="nav-tab-active">
          <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10" /></svg>
          <span>Stock</span>
          <span v-if="noSkuCount" class="absolute top-1.5 right-3 px-1.5 py-0.2 rounded-full bg-rose-600 text-white text-[10px] font-bold leading-tight">{{ noSkuCount }}</span>
        </router-link>
        <router-link to="/needs-pricing" class="nav-tab relative" active-class="nav-tab-active">
          <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M7 7h.01M7 3h5a1.99 1.99 0 011.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.99 1.99 0 013 12V7a4 4 0 014-4z" /></svg>
          <span>Pricing</span>
          <span v-if="unpricedCount" class="absolute top-1.5 right-2.5 px-1.5 py-0.2 rounded-full bg-amber-600 text-white text-[10px] font-bold leading-tight">{{ unpricedCount }}</span>
        </router-link>
        <router-link to="/sales" class="nav-tab" active-class="nav-tab-active">
          <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M9 14l6-6m-5.5.5h.01m4.99 5h.01M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16l3.5-2 3.5 2 3.5-2 3.5 2z" /></svg>
          <span>Sales</span>
        </router-link>
        <button type="button" @click="moreOpen = !moreOpen" class="nav-tab" :class="{ 'nav-tab-active': moreOpen }">
          <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M4 6h4v4H4V6zm6 0h4v4h-4V6zm6 0h4v4h-4V6zM4 14h4v4H4v-4zm6 0h4v4h-4v-4zm6 0h4v4h-4v-4z" /></svg>
          <span>More</span>
        </button>
      </div>
    </nav>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
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

const moreOpen = ref(false)
const paired = computed(() => !!device.deviceToken)
const online = ref(typeof navigator === 'undefined' ? true : navigator.onLine)
const unpricedCount = computed(() => inventory.products.filter(p => !p.priced).length)
const noSkuCount = computed(() => inventory.products.filter(p => p.no_sku || p.noSku).length)

function updateOnline() {
  online.value = typeof navigator === 'undefined' ? true : navigator.onLine
  if (online.value) {
    mutations.flush().catch(() => {})
    scans.flush().catch(() => {})
  }
}

function onVisibilityChange() {
  if (document.visibilityState === 'visible' && navigator.onLine) {
    mutations.flush().catch(() => {})
    scans.flush().catch(() => {})
  }
}

onMounted(async () => {
  window.addEventListener('online', updateOnline)
  window.addEventListener('offline', updateOnline)
  document.addEventListener('visibilitychange', onVisibilityChange)
  await Promise.all([mutations.load(), scans.load(), inventory.fetchProducts().catch(() => {})])
  mutations.flush().catch(() => {})
  scans.flush().catch(() => {})
})

onUnmounted(() => {
  window.removeEventListener('online', updateOnline)
  window.removeEventListener('offline', updateOnline)
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<style scoped>
.nav-tab {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
  font-size: 11px;
  font-weight: 600;
  color: #64748b;
  text-decoration: none;
  background: transparent;
  border: 0;
  border-radius: 12px;
  padding: 4px 2px;
}
.nav-tab-active {
  color: #1d4ed8;
  background: #eff6ff;
}
</style>
