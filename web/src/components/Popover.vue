<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, useId, watch } from 'vue'

// A button that opens a panel. role "menu" gets arrow key navigation
// between [role=menuitem] children, "dialog" is for panels with form controls.
// fixed places the panel against the window, for buttons inside a box that
// scrolls or clips (overflow), and opens it upward when it doesn't fit below.
const props = withDefaults(
  defineProps<{
    label?: string
    role?: 'menu' | 'dialog'
    align?: 'left' | 'right'
    buttonClass?: string | Record<string, boolean> | (string | Record<string, boolean>)[]
    panelClass?: string
    fixed?: boolean
  }>(),
  { role: 'menu', align: 'left' },
)

const open = ref(false)
const root = ref<HTMLElement>()
const button = ref<HTMLButtonElement>()
const panel = ref<HTMLElement>()
const panelId = useId()
const pos = ref<Record<string, string>>({})

function items() {
  return Array.from(panel.value?.querySelectorAll<HTMLElement>('[role="menuitem"]:not([disabled])') ?? [])
}

function place() {
  const b = button.value?.getBoundingClientRect()
  const p = panel.value
  if (!b || !p) return
  const gap = 6
  const h = p.offsetHeight
  const below = b.bottom + gap
  const top = below + h > window.innerHeight - 8 && b.top - gap - h >= 8 ? b.top - gap - h : below
  pos.value =
    props.align === 'right'
      ? { top: `${top}px`, right: `${document.documentElement.clientWidth - b.right}px` }
      : { top: `${top}px`, left: `${b.left}px` }
}

async function show() {
  open.value = true
  await nextTick()
  if (props.fixed) place()
  if (props.role === 'menu') items()[0]?.focus()
  else panel.value?.querySelector<HTMLElement>('input, button, a')?.focus()
}

function close(focus = true) {
  if (!open.value) return
  open.value = false
  if (focus) button.value?.focus()
}

function onPanelKey(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.preventDefault()
    close()
    return
  }
  if (props.role !== 'menu') return
  const list = items()
  const i = list.indexOf(document.activeElement as HTMLElement)
  let next = -1
  if (e.key === 'ArrowDown') next = (i + 1) % list.length
  else if (e.key === 'ArrowUp') next = (i - 1 + list.length) % list.length
  else if (e.key === 'Home') next = 0
  else if (e.key === 'End') next = list.length - 1
  else if (e.key === 'Tab') close(false)
  if (next >= 0) {
    e.preventDefault()
    list[next]?.focus()
  }
}

function onButtonKey(e: KeyboardEvent) {
  if (props.role === 'menu' && e.key === 'ArrowDown' && !open.value) {
    e.preventDefault()
    show()
  }
}

function onDocDown(e: PointerEvent) {
  if (root.value && !root.value.contains(e.target as Node)) close(false)
}

// a fixed panel would stay put while the page moves under it
function onMove(e: Event) {
  if (e.target instanceof Node && panel.value?.contains(e.target)) return
  close(false)
}

watch(open, (v) => {
  if (v) {
    document.addEventListener('pointerdown', onDocDown)
    if (props.fixed) {
      window.addEventListener('scroll', onMove, true)
      window.addEventListener('resize', onMove)
    }
  } else {
    document.removeEventListener('pointerdown', onDocDown)
    window.removeEventListener('scroll', onMove, true)
    window.removeEventListener('resize', onMove)
  }
})
onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onDocDown)
  window.removeEventListener('scroll', onMove, true)
  window.removeEventListener('resize', onMove)
})

defineExpose({ close })
</script>

<template>
  <div ref="root" class="relative">
    <button
      ref="button"
      type="button"
      :aria-label="label"
      :aria-haspopup="role === 'menu' ? 'menu' : 'dialog'"
      :aria-expanded="open"
      :aria-controls="open ? panelId : undefined"
      :class="buttonClass"
      @click="open ? close() : show()"
      @keydown="onButtonKey"
    >
      <slot name="trigger" :open="open" />
    </button>
    <div
      v-if="open"
      :id="panelId"
      ref="panel"
      :role="role"
      :aria-label="label"
      class="z-40 rounded-[10px] border border-line bg-white p-1.5 shadow-[0_8px_24px_rgba(21,23,28,0.12)]"
      :class="[fixed ? 'fixed' : ['absolute top-[calc(100%+6px)]', align === 'right' ? 'right-0' : 'left-0'], panelClass]"
      :style="fixed ? pos : undefined"
      @keydown="onPanelKey"
    >
      <slot :close="close" />
    </div>
  </div>
</template>
