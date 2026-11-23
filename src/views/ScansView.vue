<template>
  <div>
    <div class="tabs">
      <button :class="{ active: tab === 'scan' }" @click="tab = 'scan'">Scan</button>
      <button :class="{ active: tab === 'queue' }" @click="tab = 'queue'">Queue</button>
    </div>
    <ScanView v-show="tab === 'scan'" />
    <QueueView v-show="tab === 'queue'" />
  </div>
</template>

<script setup>
import { ref } from 'vue'
import ScanView from './ScanView.vue'
import QueueView from './QueueView.vue'

const tab = ref('scan')
</script>

<style scoped>
.tabs { display: flex; gap: 8px; padding: 0 0 8px; max-width: 600px; margin: 0 auto; }
.tabs button { flex: 1; padding: 10px; border-radius: 8px; }
.tabs button.active { background: #1f3a5f; color: #fff; border-color: #1f3a5f; }
</style>
