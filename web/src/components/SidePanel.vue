<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import Icon from '@/components/Icon.vue'

// A panel that slides in from the right, over the page.
defineProps<{ title: string }>()
const emit = defineEmits<{ close: [] }>()
const panel = ref<HTMLElement>()
let back: HTMLElement | null = null

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}
onMounted(async () => {
  back = document.activeElement as HTMLElement | null
  window.addEventListener('keydown', onKey)
  await nextTick()
  panel.value?.querySelector<HTMLElement>('[autofocus], input, button:not([data-close])')?.focus()
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  back?.focus?.()
})
</script>

<template>
  <Teleport to="body">
    <div class="fixed inset-0 z-40 bg-ink/20" aria-hidden="true" @click="emit('close')" />
    <section
      ref="panel"
      role="dialog"
      aria-modal="true"
      aria-labelledby="panel-title"
      class="fixed inset-y-0 right-0 z-50 flex w-full max-w-[440px] flex-col border-l border-line bg-white shadow-[-8px_0_32px_rgba(21,23,28,0.1)]"
    >
      <header class="flex items-center gap-3 border-b border-line py-4 pr-4 pl-6">
        <div class="flex min-w-0 flex-1 items-center gap-3">
          <slot name="head">
            <h2 id="panel-title" class="m-0 text-[17px] font-semibold">{{ title }}</h2>
          </slot>
        </div>
        <button type="button" data-close class="icon-btn size-9" aria-label="Close" @click="emit('close')">
          <Icon name="x" :size="18" />
        </button>
      </header>
      <div class="flex flex-1 flex-col gap-5 overflow-y-auto p-6">
        <slot />
      </div>
      <footer v-if="$slots.footer" class="flex flex-wrap items-center gap-3 border-t border-line-soft px-6 py-3.5">
        <slot name="footer" />
      </footer>
    </section>
  </Teleport>
</template>
