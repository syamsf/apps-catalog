<script setup lang="ts">
import { computed } from 'vue'
import { initials } from '@/api'

const props = withDefaults(defineProps<{ name: string; size?: number; label?: string }>(), { size: 32 })
const text = computed(() => initials(props.name || '?'))
</script>

<template>
  <span
    :role="label ? 'img' : undefined"
    :aria-label="label"
    :aria-hidden="label ? undefined : 'true'"
    class="flex shrink-0 items-center justify-center rounded-full bg-accent-soft font-semibold text-accent-ink"
    :style="{ width: `${size}px`, height: `${size}px`, fontSize: `${Math.max(10, Math.round(size * 0.35))}px` }"
    >{{ text }}</span
  >
</template>
