<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import Icon from '@/components/Icon.vue'

export interface SelectOption {
  value: string
  label: string
  // second line, e.g. an email
  hint?: string
}

const props = defineProps<{
  options: SelectOption[]
  id?: string
  placeholder?: string
  invalid?: boolean
  disabled?: boolean
}>()
const model = defineModel<string>({ required: true })
const emit = defineEmits<{ change: [value: string] }>()

const open = ref(false)
const active = ref(-1)
const root = ref<HTMLElement>()
const list = ref<HTMLElement>()
const button = ref<HTMLButtonElement>()
const listId = computed(() => (props.id ?? 'select') + '-list')

const current = computed(() => props.options.find((o) => o.value === model.value))

function show() {
  if (props.disabled || !props.options.length) return
  open.value = true
  active.value = Math.max(0, props.options.findIndex((o) => o.value === model.value))
  nextTick(scrollActive)
}

function hide(focus = false) {
  open.value = false
  if (focus) button.value?.focus()
}

function pick(i: number) {
  const o = props.options[i]
  if (!o) return
  if (o.value !== model.value) {
    model.value = o.value
    emit('change', o.value)
  }
  hide(true)
}

function move(to: number) {
  const n = props.options.length
  active.value = Math.min(n - 1, Math.max(0, to))
  nextTick(scrollActive)
}

function scrollActive() {
  list.value?.querySelector<HTMLElement>(`[data-i="${active.value}"]`)?.scrollIntoView({ block: 'nearest' })
}

let typed = ''
let typedAt = 0
function typeAhead(ch: string) {
  const now = Date.now()
  typed = now - typedAt < 600 ? typed + ch.toLowerCase() : ch.toLowerCase()
  typedAt = now
  const i = props.options.findIndex((o) => o.label.toLowerCase().startsWith(typed))
  if (i >= 0) {
    if (!open.value) show()
    move(i)
  }
}

function onKey(e: KeyboardEvent) {
  if (props.disabled) return
  const n = props.options.length
  switch (e.key) {
    case 'ArrowDown':
    case 'ArrowUp':
      e.preventDefault()
      if (!open.value) show()
      else move(active.value + (e.key === 'ArrowDown' ? 1 : -1))
      return
    case 'Home':
    case 'End':
      if (!open.value) return
      e.preventDefault()
      move(e.key === 'Home' ? 0 : n - 1)
      return
    case 'Enter':
    case ' ':
      e.preventDefault()
      if (open.value) pick(active.value)
      else show()
      return
    case 'Escape':
      if (open.value) {
        e.preventDefault()
        hide(true)
      }
      return
    case 'Tab':
      hide()
      return
  }
  if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) typeAhead(e.key)
}

function onDocDown(e: PointerEvent) {
  if (root.value && !root.value.contains(e.target as Node)) hide()
}

watch(open, (v) => {
  if (v) document.addEventListener('pointerdown', onDocDown)
  else document.removeEventListener('pointerdown', onDocDown)
})
onBeforeUnmount(() => document.removeEventListener('pointerdown', onDocDown))
</script>

<template>
  <div ref="root" class="relative">
    <button
      :id="id"
      ref="button"
      type="button"
      role="combobox"
      aria-haspopup="listbox"
      :aria-expanded="open"
      :aria-controls="listId"
      :aria-activedescendant="open && active >= 0 ? `${listId}-${active}` : undefined"
      :aria-invalid="invalid || undefined"
      :disabled="disabled"
      class="field flex items-center gap-2 text-left disabled:cursor-not-allowed disabled:bg-chip disabled:text-disabled"
      :class="open ? 'border-accent' : 'hover:border-[#aeb3ba]'"
      @click="open ? hide() : show()"
      @keydown="onKey"
    >
      <span class="min-w-0 flex-1 truncate" :class="current ? '' : 'text-faint'">
        {{ current?.label ?? placeholder ?? '' }}
      </span>
      <Icon name="chevron" :size="16" class="shrink-0 text-subtle transition-transform" :class="{ 'rotate-180': open }" />
    </button>

    <ul
      v-show="open"
      :id="listId"
      ref="list"
      role="listbox"
      tabindex="-1"
      class="absolute top-[calc(100%+4px)] right-0 left-0 z-30 m-0 max-h-72 list-none overflow-auto rounded-lg border border-line bg-white p-1 shadow-[0_8px_24px_rgba(21,23,28,0.12)]"
    >
      <li
        v-for="(o, i) in options"
        :id="`${listId}-${i}`"
        :key="o.value"
        :data-i="i"
        role="option"
        :aria-selected="o.value === model"
        class="flex cursor-pointer items-center gap-2 rounded-md px-2.5 py-2 text-sm"
        :class="[i === active ? 'bg-accent-soft' : '', o.value === model ? 'font-medium text-accent-ink' : 'text-ink']"
        @pointerenter="active = i"
        @pointerdown.prevent
        @click="pick(i)"
      >
        <span class="flex min-w-0 flex-1 flex-col">
          <span class="truncate">{{ o.label }}</span>
          <span v-if="o.hint" class="truncate text-xs font-normal text-subtle">{{ o.hint }}</span>
        </span>
        <Icon v-if="o.value === model" name="check" :size="16" class="shrink-0 text-accent" />
      </li>
    </ul>
  </div>
</template>
