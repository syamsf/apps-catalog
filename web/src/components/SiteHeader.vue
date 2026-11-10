<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import AccountMenu from '@/components/AccountMenu.vue'
import Icon from '@/components/Icon.vue'
import Logo from '@/components/Logo.vue'
import { type SearchKey, activeOption, sendSearchKey } from '@/searchNav'
import { isAdmin } from '@/session'

const props = defineProps<{ search?: boolean }>()

const route = useRoute()
const router = useRouter()
const q = ref(String(route.query.q ?? ''))
const input = ref<HTMLInputElement>()

watch(
  () => route.query.q,
  (v) => {
    if (String(v ?? '') !== q.value.trim()) q.value = String(v ?? '')
  },
)

let timer: ReturnType<typeof setTimeout> | undefined
function apply() {
  const query = { ...route.query, q: q.value.trim() || undefined }
  router.replace({ name: 'catalog', query })
}
function onInput() {
  clearTimeout(timer)
  timer = setTimeout(apply, 200)
}
function clear() {
  q.value = ''
  clearTimeout(timer)
  apply()
  input.value?.focus()
}

// arrows and Enter move through and open the results in the catalog
function onSearchKey(e: KeyboardEvent) {
  const text = q.value.trim()
  if (e.isComposing || e.ctrlKey || e.metaKey || e.altKey || !text) return
  let k: SearchKey | undefined
  if (e.key === 'ArrowDown' && !e.shiftKey) k = 'down'
  else if (e.key === 'ArrowUp' && !e.shiftKey) k = 'up'
  else if (e.key === 'Enter') k = e.shiftKey ? 'shift-enter' : 'enter'
  if (!k) return
  // Enter searches right away instead of waiting for the debounce
  if ((k === 'enter' || k === 'shift-enter') && text !== String(route.query.q ?? '')) {
    clearTimeout(timer)
    apply()
  }
  if (sendSearchKey(k, text)) e.preventDefault()
}

// "/" jumps to the search box, like most sites with search
function onKey(e: KeyboardEvent) {
  if (e.key !== '/' || e.ctrlKey || e.metaKey || e.altKey) return
  const t = e.target as HTMLElement | null
  if (t && (t.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(t.tagName))) return
  e.preventDefault()
  input.value?.focus()
}
onMounted(() => {
  if (props.search) window.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => {
  clearTimeout(timer)
  window.removeEventListener('keydown', onKey)
})
</script>

<template>
  <header class="border-b border-line bg-white">
    <div class="mx-auto flex max-w-[1440px] flex-wrap items-center gap-x-6 gap-y-4 px-4 py-3.5 sm:px-8 xl:px-10">
      <Logo />
      <div v-if="search" class="relative max-w-[560px] flex-[1_1_320px]">
        <label for="app-search" class="sr-only">Search apps</label>
        <Icon name="search" class="pointer-events-none absolute top-[13px] left-3.5 text-subtle" />
        <input
          id="app-search"
          ref="input"
          v-model="q"
          type="search"
          placeholder="Search apps, teams, or hosts"
          aria-keyshortcuts="/"
          aria-controls="app-results"
          :aria-activedescendant="activeOption || undefined"
          class="h-11 w-full rounded-[10px] border border-field bg-white pr-11 pl-[42px] text-[15px] text-ink placeholder:text-faint [&::-webkit-search-cancel-button]:hidden"
          @input="onInput"
          @keydown.esc="q && clear()"
          @keydown="onSearchKey"
        />
        <button v-if="q" type="button" aria-label="Clear search" class="icon-btn absolute top-1.5 right-1.5" @click="clear">
          <Icon name="x" :size="16" />
        </button>
        <kbd
          v-else
          aria-hidden="true"
          class="pointer-events-none absolute top-[11px] right-3 rounded-md bg-chip px-[7px] py-0.5 font-mono text-xs text-subtle"
          >/</kbd
        >
      </div>
      <div class="ml-auto flex flex-wrap items-center gap-2.5">
        <template v-if="isAdmin">
          <RouterLink to="/admin" class="btn btn-secondary">Admin</RouterLink>
          <RouterLink to="/admin/apps/new" class="btn btn-primary">Add app</RouterLink>
        </template>
        <AccountMenu />
      </div>
    </div>
  </header>
</template>
