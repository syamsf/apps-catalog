<script setup lang="ts">
import { computed, useId } from 'vue'
import Icon from '@/components/Icon.vue'
import Popover from '@/components/Popover.vue'

export interface FilterOption {
  key: string
  label: string
  // apps that would match, given the other filters
  count: number
}

const props = defineProps<{
  label: string
  options: FilterOption[]
  // apps shown right now, for the close button
  results: number
}>()
const model = defineModel<string[]>({ required: true })
const id = useId()

const summary = computed(() => {
  const picked = props.options.filter((o) => model.value.includes(o.key))
  if (!picked.length) return ''
  return picked.length === 1 ? picked[0]!.label : `${picked[0]!.label} +${picked.length - 1}`
})

function toggle(key: string) {
  model.value = model.value.includes(key) ? model.value.filter((k) => k !== key) : [...model.value, key]
}
</script>

<template>
  <Popover
    role="dialog"
    :label="`Filter by ${label.toLowerCase()}`"
    panel-class="w-64"
    :button-class="[
      'flex h-9 cursor-pointer items-center gap-1.5 rounded-lg border pr-2.5 pl-3 text-[13.5px] font-medium',
      summary ? 'border-[#b9c8ee] bg-accent-soft text-accent-ink' : 'border-field bg-white text-ink hover:border-[#aeb3ba]',
    ]"
  >
    <template #trigger="{ open }">
      <span>{{ label }}<template v-if="summary">:</template></span>
      <span v-if="summary" class="font-semibold">{{ summary }}</span>
      <Icon name="chevron" :size="14" class="transition-transform" :class="[{ 'rotate-180': open }, summary ? '' : 'text-subtle']" />
    </template>
    <template #default="{ close }">
      <fieldset class="m-0 flex max-h-80 flex-col overflow-auto border-0 p-0">
        <legend class="sr-only">{{ label }}</legend>
        <label
          v-for="o in options"
          :key="o.key"
          :for="`${id}-${o.key}`"
          class="flex min-h-9 cursor-pointer items-center gap-2.5 rounded-md px-2.5 text-sm hover:bg-chip"
          :class="{ 'text-disabled': !o.count && !model.includes(o.key) }"
        >
          <input
            :id="`${id}-${o.key}`"
            type="checkbox"
            class="check"
            :checked="model.includes(o.key)"
            @change="toggle(o.key)"
          />
          <span class="min-w-0 flex-1 truncate">{{ o.label }}</span>
          <span class="text-[12.5px] text-faint">{{ o.count }}</span>
        </label>
        <p v-if="!options.length" class="m-0 px-2.5 py-2 text-sm text-subtle">Nothing to filter by yet.</p>
      </fieldset>
      <div class="menu-sep" />
      <div class="flex items-center justify-between gap-2 px-1 pt-0.5 pb-0.5">
        <button
          type="button"
          class="btn btn-ghost btn-sm px-2.5 text-subtle"
          :disabled="!model.length"
          :class="{ invisible: !model.length }"
          @click="model = []"
        >
          Clear
        </button>
        <button type="button" class="btn btn-primary btn-sm" @click="close()">
          Show {{ results }} {{ results === 1 ? 'app' : 'apps' }}
        </button>
      </div>
    </template>
  </Popover>
</template>
