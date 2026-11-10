<script setup lang="ts">
import { RouterLink } from 'vue-router'
import Icon from '@/components/Icon.vue'
import { portalName } from '@/session'

withDefaults(defineProps<{ to?: string; large?: boolean }>(), { to: '/' })
</script>

<template>
  <RouterLink :to="to" class="flex items-center gap-2.5 text-ink no-underline hover:text-ink">
    <span
      class="flex items-center justify-center bg-accent text-white"
      :class="large ? 'size-9 rounded-[9px]' : 'size-8 rounded-lg'"
    >
      <Icon name="grid" :size="large ? 20 : 18" />
    </span>
    <span class="font-semibold" :class="large ? 'text-lg' : 'text-[17px]'">{{ portalName }}</span>
  </RouterLink>
</template>
