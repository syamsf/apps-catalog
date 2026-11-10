<script lang="ts">
import { type TablerSet, api } from '@/api'

// Dashboard Icons, https://dashboardicons.com, loaded from jsDelivr
const DASH = 'https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons@main'

type DashIcon = { n: string; url: string; type: string }

// loaded once per page, the picker opens and closes often
let tablerLoad: Promise<TablerSet> | undefined
let dashLoad: Promise<DashIcon[]> | undefined

function loadTabler() {
  tablerLoad ??= api.tablerIcons().catch((e) => {
    tablerLoad = undefined
    throw e
  })
  return tablerLoad
}

function loadDash() {
  dashLoad ??= fetch(`${DASH}/tree.json`)
    .then((r) => {
      if (!r.ok) throw new Error(`HTTP ${r.status}`)
      return r.json() as Promise<{ svg?: string[]; png?: string[] }>
    })
    .then((tree) => {
      const out = new Map<string, DashIcon>()
      for (const f of tree.svg ?? []) {
        const n = f.replace(/\.svg$/, '')
        out.set(n, { n, url: `${DASH}/svg/${f}`, type: 'image/svg+xml' })
      }
      for (const f of tree.png ?? []) {
        const n = f.replace(/\.png$/, '')
        if (!out.has(n)) out.set(n, { n, url: `${DASH}/png/${f}`, type: 'image/png' })
      }
      return [...out.values()].sort((a, b) => a.n.localeCompare(b.n))
    })
    .catch((e) => {
      dashLoad = undefined
      throw e
    })
  return dashLoad
}
</script>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Icon from '@/components/Icon.vue'
import { errorText } from '@/api'

const props = defineProps<{
  // starting search, usually the app name
  query: string
  // Tabler icon in use
  current: string
  // checks and keeps a picked file, returns an error message or ''
  onFile: (file: File) => Promise<string>
}>()
const emit = defineEmits<{ close: []; tabler: [name: string] }>()

const PAGE = 120
const tab = ref<'tabler' | 'dash'>('tabler')
const q = ref(props.query.trim().toLowerCase())
const limit = ref(PAGE)
const input = ref<HTMLInputElement>()

const tabler = ref<TablerSet['icons']>([])
const tablerError = ref('')
const dash = ref<DashIcon[]>([])
const dashError = ref('')
const dashLoading = ref(false)
const busy = ref('')
const pickError = ref('')

async function getTabler() {
  tablerError.value = ''
  try {
    tabler.value = (await loadTabler()).icons
  } catch (e) {
    tablerError.value = errorText(e, 'Could not load the Tabler icons.')
  }
}
async function getDash() {
  dashError.value = ''
  dashLoading.value = true
  try {
    dash.value = await loadDash()
  } catch {
    dashError.value = "This browser can't reach cdn.jsdelivr.net, where Dashboard Icons are served from."
  } finally {
    dashLoading.value = false
  }
}

// "Argo CD" finds argo-cd, "kube" finds kubernetes
const words = computed(() => q.value.toLowerCase().split(/[\s_-]+/).filter(Boolean))
function score(name: string, extra = '') {
  const w = words.value
  if (!w.length) return 1
  const joined = w.join('-')
  if (name === joined || name === `brand-${joined}`) return 4
  if (name.startsWith(joined) || name.startsWith(`brand-${joined}`)) return 3
  if (name.includes(joined) || name.includes(w.join(''))) return 2
  const hay = `${name} ${extra}`
  return w.every((x) => hay.includes(x)) ? 1 : 0
}
function rank<T>(list: T[], key: (x: T) => [string, string?]) {
  if (!words.value.length) return list
  const out: [number, T][] = []
  for (const x of list) {
    const [n, extra] = key(x)
    const s = score(n, extra)
    if (s) out.push([s, x])
  }
  return out.sort((a, b) => b[0] - a[0]).map(([, x]) => x)
}
const tablerHits = computed(() => rank(tabler.value, (i) => [i.n, `${i.t ?? ''} ${(i.c ?? '').toLowerCase()}`]))
const dashHits = computed(() => rank(dash.value, (i) => [i.n]))
const hits = computed(() => (tab.value === 'tabler' ? tablerHits.value : dashHits.value))

watch([q, tab], () => (limit.value = PAGE))

// open on the set that has the app, unless someone already picked a tab
const tabTouched = ref(false)
watch([tablerHits, dashHits], () => {
  if (tabTouched.value || !tabler.value.length || !dash.value.length || q.value !== props.query.trim().toLowerCase()) return
  if (!tablerHits.value.length && dashHits.value.length) tab.value = 'dash'
  tabTouched.value = true
})
watch(tab, (t) => {
  pickError.value = ''
  if (t === 'dash' && !dash.value.length) getDash()
})

async function pickDash(i: DashIcon) {
  if (busy.value) return
  busy.value = i.n
  pickError.value = ''
  try {
    const r = await fetch(i.url)
    if (!r.ok) throw new Error(`HTTP ${r.status}`)
    const blob = await r.blob()
    const file = new File([blob], `${i.n}.${i.type === 'image/png' ? 'png' : 'svg'}`, { type: i.type })
    const msg = await props.onFile(file)
    if (msg) pickError.value = msg
    else emit('close')
  } catch {
    pickError.value = `Could not download ${i.n}. Try again, or upload the file yourself.`
  } finally {
    busy.value = ''
  }
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}
onMounted(async () => {
  window.addEventListener('keydown', onKey)
  getTabler()
  // counts on both tabs help find the right one
  getDash()
  await nextTick()
  input.value?.focus()
  input.value?.select()
})
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))

const fmt = (n: number) => n.toLocaleString('en')
</script>

<template>
  <Teleport to="body">
    <div class="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-ink/50 px-4 pt-[8vh] pb-4" @click.self="emit('close')">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="picker-title"
        class="flex max-h-[84vh] w-full max-w-[760px] flex-col rounded-[14px] bg-white shadow-[0_20px_50px_rgba(21,23,28,0.25)]"
      >
        <div class="flex flex-col gap-3.5 border-b border-line-soft px-6 pt-5 pb-4">
          <div class="flex items-center justify-between gap-3">
            <h2 id="picker-title" class="m-0 text-lg font-semibold">Choose an icon</h2>
            <button type="button" aria-label="Close" class="icon-btn" @click="emit('close')"><Icon name="x" :size="18" /></button>
          </div>
          <div class="relative">
            <label for="picker-q" class="sr-only">Search icons</label>
            <Icon name="search" class="pointer-events-none absolute top-[13px] left-3.5 text-subtle" />
            <input
              id="picker-q"
              ref="input"
              v-model="q"
              type="search"
              placeholder="Search, like grafana or database"
              autocomplete="off"
              spellcheck="false"
              class="field pl-[42px]"
            />
          </div>
          <div role="tablist" aria-label="Icon sets" class="flex gap-1">
            <button
              v-for="t in [
                { key: 'tabler', label: 'Tabler icons', count: tablerHits.length, ready: !!tabler.length },
                { key: 'dash', label: 'Dashboard Icons', count: dashHits.length, ready: !!dash.length },
              ] as const"
              :key="t.key"
              type="button"
              role="tab"
              :aria-selected="tab === t.key"
              class="flex h-9 cursor-pointer items-center gap-2 rounded-lg px-3 text-[13.5px] font-medium text-subtle hover:bg-chip aria-selected:bg-[#e9e7e1] aria-selected:text-ink"
              @click="((tab = t.key), (tabTouched = true))"
            >
              {{ t.label }}
              <span v-if="t.ready" class="font-mono text-xs font-normal">{{ fmt(t.count) }}</span>
            </button>
          </div>
        </div>

        <div class="flex min-h-[280px] flex-col gap-3 overflow-y-auto px-6 py-4" role="tabpanel">
          <p v-if="pickError" role="alert" class="status-error">{{ pickError }}</p>

          <template v-if="tab === 'tabler'">
            <div v-if="tablerError" class="flex flex-col items-start gap-2.5">
              <p class="status-error w-full">{{ tablerError }}</p>
              <button type="button" class="btn btn-secondary btn-sm" @click="getTabler">Try again</button>
            </div>
            <p v-else-if="!tabler.length" class="m-0 text-sm text-subtle">Loading icons...</p>
          </template>
          <template v-else>
            <div v-if="dashError" class="flex flex-col items-start gap-2.5">
              <p class="status-error w-full">{{ dashError }}</p>
              <div class="flex flex-wrap gap-2">
                <button type="button" class="btn btn-secondary btn-sm" @click="getDash">Try again</button>
                <button type="button" class="btn btn-secondary btn-sm" @click="tab = 'tabler'">Use Tabler icons</button>
              </div>
            </div>
            <p v-else-if="dashLoading && !dash.length" class="m-0 text-sm text-subtle">Loading icons...</p>
          </template>

          <p
            v-if="!hits.length && ((tab === 'tabler' && tabler.length) || (tab === 'dash' && dash.length))"
            class="m-0 py-6 text-center text-sm text-muted"
          >
            No icons match "{{ q }}".
            <button type="button" class="link cursor-pointer" @click="q = ''">Show all</button>
          </p>

          <ul v-if="hits.length" class="m-0 grid list-none grid-cols-[repeat(auto-fill,minmax(84px,1fr))] gap-1.5 p-0">
            <template v-if="tab === 'tabler'">
              <li v-for="i in (hits as TablerSet['icons']).slice(0, limit)" :key="i.n">
                <button
                  type="button"
                  :aria-pressed="current === i.n"
                  :title="i.n"
                  class="flex h-[78px] w-full cursor-pointer flex-col items-center justify-center gap-1.5 rounded-lg border border-transparent px-1 text-ink hover:border-line hover:bg-hover aria-pressed:border-accent aria-pressed:bg-accent-soft"
                  @click="(emit('tabler', i.n), emit('close'))"
                >
                  <svg
                    xmlns="http://www.w3.org/2000/svg"
                    viewBox="0 0 24 24"
                    width="26"
                    height="26"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="1.8"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    aria-hidden="true"
                    v-html="i.b"
                  />
                  <span class="w-full truncate text-center text-[11px] text-subtle">{{ i.n }}</span>
                </button>
              </li>
            </template>
            <template v-else>
              <li v-for="i in (hits as DashIcon[]).slice(0, limit)" :key="i.n">
                <button
                  type="button"
                  :title="i.n"
                  :disabled="!!busy"
                  class="flex h-[78px] w-full cursor-pointer flex-col items-center justify-center gap-1.5 rounded-lg border border-transparent px-1 hover:border-line hover:bg-hover disabled:cursor-wait"
                  :class="{ 'opacity-50': busy && busy !== i.n }"
                  @click="pickDash(i)"
                >
                  <img :src="i.url" alt="" loading="lazy" decoding="async" class="size-8 object-contain" />
                  <span class="w-full truncate text-center text-[11px] text-subtle">{{ busy === i.n ? 'Adding...' : i.n }}</span>
                </button>
              </li>
            </template>
          </ul>
          <button v-if="hits.length > limit" type="button" class="btn btn-secondary btn-sm self-center" @click="limit += PAGE * 2">
            Show more ({{ fmt(hits.length - limit) }} left)
          </button>
        </div>

        <p class="m-0 border-t border-line-soft px-6 py-3 text-[12px] leading-normal text-subtle">
          <template v-if="tab === 'tabler'">
            Icons from <a href="https://tabler.io/icons" target="_blank" rel="noopener" class="link font-normal">Tabler</a>, drawn in
            white on the icon color. They come with the portal and work offline.
          </template>
          <template v-else>
            Logos from <a href="https://dashboardicons.com" target="_blank" rel="noopener" class="link font-normal">Dashboard Icons</a>.
            The one you pick is saved in the portal like an uploaded icon.
          </template>
        </p>
      </div>
    </div>
  </Teleport>
</template>
