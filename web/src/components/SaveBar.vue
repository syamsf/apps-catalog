<script setup lang="ts">
// Sticks to the bottom of the screen while a form has unsaved changes.
defineProps<{ show: boolean; label: string; detail?: string; saving?: boolean; action: string; busyAction?: string; form?: string }>()
defineEmits<{ cancel: [] }>()
</script>

<template>
  <Transition
    enter-from-class="translate-y-full"
    enter-active-class="transition-transform duration-200"
    leave-to-class="translate-y-full"
    leave-active-class="transition-transform duration-150"
  >
    <div
      v-if="show"
      class="fixed inset-x-0 bottom-0 z-30 border-t border-line bg-white shadow-[0_-4px_16px_rgba(21,23,28,0.06)]"
    >
      <div class="mx-auto flex max-w-[1440px] flex-wrap items-center gap-x-3 gap-y-2 px-4 py-3 sm:px-8 md:pl-[260px]">
        <span class="size-2 rounded-full bg-star" aria-hidden="true" />
        <span class="text-sm font-medium">{{ label }}</span>
        <span v-if="detail" class="min-w-0 truncate text-[13px] text-subtle">{{ detail }}</span>
        <span class="ml-auto flex gap-2">
          <button type="button" class="btn btn-ghost" :disabled="saving" @click="$emit('cancel')">Cancel</button>
          <button type="submit" :form="form" class="btn btn-primary px-5" :disabled="saving">
            {{ saving ? (busyAction ?? 'Saving...') : action }}
          </button>
        </span>
      </div>
    </div>
  </Transition>
</template>
