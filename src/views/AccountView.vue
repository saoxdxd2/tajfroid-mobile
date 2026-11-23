<template>
  <div class="view">
    <h1>Account</h1>
    <p>Device paired: {{ paired ? 'yes' : 'no' }}</p>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useDeviceStore } from '../stores/device'
const store = useDeviceStore()
const paired = computed(() => !!store.deviceToken)
</script>