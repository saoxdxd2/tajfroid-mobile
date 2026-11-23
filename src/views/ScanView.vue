<template>
  <div class="view">
    <h1>Scan supplier paper</h1>
    <input type="file" accept="image/*" capture="environment" @change="onFile" />
    <p v-if="status">{{ status }}</p>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useQueueStore } from '../stores/queue'
const queue = useQueueStore()
const status = ref('')
async function onFile(e) {
  const file = e.target.files?.[0]
  if (!file) return
  status.value = 'Queueing...'
  await queue.enqueue(file)
  status.value = 'Queued. Will upload when online.'
}
</script>