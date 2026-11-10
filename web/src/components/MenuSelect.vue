<script setup lang="ts">
import { computed } from 'vue'
import Icon from '@/components/Icon.vue'
import Popover from '@/components/Popover.vue'

// A compact "Label: Value" picker for toolbars.
const props = defineProps<{ label: string; options: { value: string; label: string }[]; align?: 'left' | 'right' }>()
const model = defineModel<string>({ required: true })
const current = computed(() => props.options.find((o) => o.value === model.value)?.label ?? '')
</script>

<template>
  <Popover
    :label="label"
    :align="align"
    panel-class="min-w-48 max-h-80 overflow-auto"
    button-class="flex h-10 cursor-pointer items-center gap-2 rounded-lg border border-field bg-white pr-2.5 pl-3 text-sm hover:border-[#aeb3ba]"
  >
    <template #trigger="{ open }">
      <span class="text-subtle">{{ label }}:</span>
      <span class="font-medium">{{ current }}</span>
      <Icon name="chevron" :size="14" class="text-subtle transition-transform" :class="{ 'rotate-180': open }" />
    </template>
    <template #default="{ close }">
      <button
        v-for="o in options"
        :key="o.value"
        type="button"
        role="menuitemradio"
        :aria-checked="o.value === model"
        class="menu-item"
        @click="((model = o.value), close())"
      >
        <span class="flex-1">{{ o.label }}</span>
        <Icon v-if="o.value === model" name="check" :size="16" class="text-accent" />
      </button>
    </template>
  </Popover>
</template>
