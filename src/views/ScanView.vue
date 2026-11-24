<template>
  <div class="view">
    <h1>{{ COPY.scanTitle }}</h1>
    <label for="scan-file" class="btn-primary photo-button">{{ COPY.scanTakePhoto }}</label>
    <input id="scan-file" class="sr-only" type="file" accept="image/*" capture="environment" @change="onFile" />
    <p v-if="status" class="status" role="status">{{ status }}</p>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useQueueStore } from '../stores/queue'
import { COPY } from '../lib/copy'
const queue = useQueueStore()
const status = ref('')
async function onFile(e) {
  const file = e.target.files?.[0]
  if (!file) return
  status.value = COPY.scanQueueing
  await queue.enqueue(file)
  status.value = COPY.scanSaved
}
</script>

<style scoped>
.photo-button { display: inline-flex; align-items: center; }
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}
.status { margin-top: 12px; color: #1b5e20; }
</style>
