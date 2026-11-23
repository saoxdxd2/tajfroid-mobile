<template>
  <div class="view">
    <h1>Queue</h1>
    <ul>
      <li v-for="s in items" :key="s.clientId">
        {{ s.clientId }} — {{ s.status }}
        <span v-if="s.purchaseReference"> ({{ s.purchaseReference }})</span>
        <span v-if="s.message"> — {{ s.message }}</span>
      </li>
    </ul>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useQueueStore } from '../stores/queue'
const queue = useQueueStore()
const items = computed(() => queue.items)
</script>