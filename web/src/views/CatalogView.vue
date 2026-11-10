<script lang="ts">
// search texts already reported in this page load
const reported = new Set<string>()
</script>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import AppCard from '@/components/AppCard.vue'
import AppIcon from '@/components/AppIcon.vue'
import DeleteAppModal from '@/components/DeleteAppModal.vue'
import Icon from '@/components/Icon.vue'
import SiteHeader from '@/components/SiteHeader.vue'
import { type App, api, copyText, errorText, href, isAbort, plural, primaryURL, similarApps, trackOpen } from '@/api'
import { type SearchKey, activeOption, setSearchKeyHandler } from '@/searchNav'
import { isAdmin, me, optionLabel, settings } from '@/session'

const route = useRoute()
const router = useRouter()

type View = 'all' | 'favorites' | 'start'
const q = computed(() => String(route.query.q ?? '').trim())
const category = computed(() => String(route.query.category ?? '').split(',')[0] ?? '')
const view = computed<View>(() => {
  if (category.value) return 'all'
  const v = route.query.view
  return v === 'favorites' || v === 'start' ? v : 'all'
})

const all = ref<App[]>([])
// search results from the server, the sidebar view is applied on top
const found = ref<App[]>([])
// ids of the apps this person opened last, newest first
const recent = ref<string[]>([])
const loading = ref(true)
const loaded = ref(false)
const error = ref('')

const envs = computed(() => settings.value?.environments ?? [])
const categories = computed(() => settings.value?.categories ?? [])
const myGroups = computed(() => me.value?.user.groups ?? [])
const adminEmail = computed(() => settings.value?.admin_email ?? '')

// favorites are kept in the browser, so they still work when the server doesn't answer
const FAV_KEY = 'portal.favorites'
const cachedFavs = ref<App[]>([])
function keepFavorites(apps: App[]) {
  try {
    localStorage.setItem(FAV_KEY, JSON.stringify(apps.filter((a) => a.favorite)))
  } catch {
    // private mode or storage full
  }
}
function readFavorites(): App[] {
  try {
    const v = JSON.parse(localStorage.getItem(FAV_KEY) ?? '[]')
    return Array.isArray(v) ? v : []
  } catch {
    return []
  }
}

const LAYOUT_KEY = 'portal.catalog-layout'
const layout = ref<'list' | 'cards'>('list')
try {
  if (localStorage.getItem(LAYOUT_KEY) === 'cards') layout.value = 'cards'
} catch {
  // keep the default
}
function setLayout(v: 'list' | 'cards') {
  layout.value = v
  try {
    localStorage.setItem(LAYOUT_KEY, v)
  } catch {
    // only a convenience
  }
}

const SORT_KEY = 'portal.catalog-sort'
const sort = ref<'name' | 'popular'>('name')
try {
  if (localStorage.getItem(SORT_KEY) === 'popular') sort.value = 'popular'
} catch {
  // keep the default
}
function setSort(v: 'name' | 'popular') {
  sort.value = v
  try {
    localStorage.setItem(SORT_KEY, v)
  } catch {
    // only a convenience
  }
}

// the search box types fast, so a search counts once it stayed the same for 2 seconds
let reportTimer: ReturnType<typeof setTimeout> | undefined
function reportLater(text: string) {
  clearTimeout(reportTimer)
  reportTimer = setTimeout(() => {
    const key = text.toLowerCase()
    if (q.value !== text || reported.has(key)) return
    reported.add(key)
    api.logSearch(text, found.value.length > 0)
  }, 2000)
}

// Enter pressed before the results for the typed text are in, run once they are
interface Pending {
  key: SearchKey
  text: string
  // blank tab opened during the key press, popup blockers only allow it there
  win: Window | null
}
let pending: Pending | null = null

// query text the results in found belong to
let settled = ''
let ctrl: AbortController | undefined
async function search() {
  ctrl?.abort()
  if (!q.value) {
    found.value = all.value
    settled = ''
    return
  }
  const c = (ctrl = new AbortController())
  const text = q.value
  loading.value = true
  try {
    found.value = (await api.apps({ q: text }, c.signal)).apps
    error.value = ''
    settled = text
    reportLater(text)
    runPending()
  } catch (e) {
    if (!isAbort(e)) {
      error.value = errorText(e, 'Could not load the apps.')
      dropPending()
    }
  } finally {
    if (ctrl === c) loading.value = false
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const res = await api.apps()
    all.value = res.apps
    recent.value = res.recent
    keepFavorites(res.apps)
    loaded.value = true
    await search()
  } catch (e) {
    error.value = errorText(e, 'The portal server did not answer.')
    dropPending()
    if (!loaded.value) cachedFavs.value = readFavorites()
  } finally {
    loading.value = false
  }
}
load()

watch(q, () => {
  clearTimeout(reportTimer)
  if (pending && pending.text !== q.value) dropPending()
  search()
})
onBeforeUnmount(() => {
  ctrl?.abort()
  clearTimeout(reportTimer)
  dropPending()
})

function setQuery(patch: Record<string, string | undefined>) {
  router.replace({ query: { ...route.query, ...patch } })
}
const pick = (v: View, cat = '') => setQuery({ view: v === 'all' ? undefined : v, category: cat || undefined })

function inView(a: App) {
  if (category.value) return a.category === category.value
  if (view.value === 'favorites') return a.favorite
  if (view.value === 'start') return a.start_here
  return true
}
const inSelection = computed(() => all.value.filter(inView))
const byName = (a: App, b: App) => a.name.toLowerCase().localeCompare(b.name.toLowerCase()) || a.id.localeCompare(b.id)
const shown = computed(() => {
  const list = found.value.filter(inView)
  if (sort.value === 'popular') list.sort((a, b) => b.opens_30d - a.opens_30d || byName(a, b))
  return list
})

const recentApps = computed(() =>
  recent.value
    .map((id) => all.value.find((a) => a.id === id))
    .filter((a): a is App => !!a)
    .slice(0, 5),
)

const catColors = ['#4A63A8', '#8A4B1F', '#2F7A45', '#6A4C9C', '#12707A', '#6B6A12', '#8A3F6E', '#9A3A3A']
const catLabel = (k: string) => optionLabel(categories.value, k)
const runsLabel = (k: string) => optionLabel(settings.value?.runs_on ?? [], k)

const nav = computed(() => [
  { key: 'all', label: 'All apps', count: all.value.length, active: view.value === 'all' && !category.value, go: () => pick('all') },
  {
    key: 'favorites',
    label: 'Favorites',
    count: all.value.filter((a) => a.favorite).length,
    active: view.value === 'favorites' && !category.value,
    go: () => pick('favorites'),
  },
  {
    key: 'start',
    label: 'Start here',
    count: all.value.filter((a) => a.start_here).length,
    active: view.value === 'start' && !category.value,
    go: () => pick('start'),
  },
])
const catNav = computed(() =>
  categories.value
    .map((o, i) => ({
      key: o.key,
      label: o.label,
      color: catColors[i % catColors.length],
      count: all.value.filter((a) => a.category === o.key).length,
      active: category.value === o.key,
      go: () => pick('all', o.key),
    }))
    .filter((c) => c.count > 0 || c.active),
)

const title = computed(() => {
  if (category.value) return catLabel(category.value)
  return nav.value.find((n) => n.key === view.value)!.label
})
const countText = computed(() => {
  const n = inSelection.value.length
  if (!q.value) return plural(n, 'app')
  return `${shown.value.length} of ${plural(n, 'app')}`
})

// every check failing at once points at the portal, not the apps
const checksDown = computed(() => {
  const checked = all.value.filter((a) => a.status)
  if (checked.length < 2 || checked.some((a) => a.status!.up)) return null
  const dns = checked.every((a) => /no such host|lookup /i.test(a.status!.error))
  return { dns }
})

const favBusy = ref(new Set<string>())
async function toggleFav(app: App) {
  if (favBusy.value.has(app.id)) return
  const on = !app.favorite
  const set = (v: boolean) => {
    for (const list of [all.value, found.value]) {
      const a = list.find((x) => x.id === app.id)
      if (a) a.favorite = v
    }
  }
  favBusy.value.add(app.id)
  set(on)
  try {
    await api.favorite(app.id, on)
    keepFavorites(all.value)
  } catch (e) {
    set(!on)
    error.value = errorText(e, 'Could not update favorites.')
  } finally {
    favBusy.value.delete(app.id)
  }
}

const notice = ref('')
let noticeTimer: ReturnType<typeof setTimeout> | undefined
function say(text: string) {
  notice.value = text
  clearTimeout(noticeTimer)
  noticeTimer = setTimeout(() => (notice.value = ''), 2500)
}
onBeforeUnmount(() => clearTimeout(noticeTimer))

async function copyURL(app: App, url: string) {
  await copyText(url)
  say(`Copied the URL of ${app.name}.`)
}

const deleting = ref<App | null>(null)
function onDeleted() {
  const a = deleting.value
  if (!a) return
  all.value = all.value.filter((x) => x.id !== a.id)
  found.value = found.value.filter((x) => x.id !== a.id)
  deleting.value = null
  say(`${a.name} was deleted from the catalog.`)
}

const openURL = (a: App) => {
  const u = primaryURL(a, envs.value)
  return u ? href(u) : ''
}
const detailsTo = (a: App) => `/apps/${encodeURIComponent(a.id)}`

// highlighted result for the keys in the search box, -1 when there is no query
const active = ref(-1)
const rowId = (a: App) => `app-row-${a.id}`
watch(
  () => [q.value, shown.value.map((a) => a.id).join(',')],
  () => (active.value = q.value && shown.value.length ? 0 : -1),
  { immediate: true },
)
watch(
  () => (active.value >= 0 && shown.value[active.value] ? rowId(shown.value[active.value]!) : ''),
  (id) => (activeOption.value = id),
  { immediate: true },
)

function dropPending() {
  pending?.win?.close()
  pending = null
}

function runPending() {
  const p = pending
  if (!p || p.text !== q.value) return
  pending = null
  const a = shown.value[0]
  const url = a && p.key === 'enter' ? openURL(a) : ''
  if (a && url) {
    if (p.win) p.win.location.href = url
    else window.open(url, '_blank', 'noopener')
    trackOpen(a.id)
    return
  }
  p.win?.close()
  if (a) router.push(detailsTo(a))
}

function openApp(k: SearchKey, a: App) {
  const url = openURL(a)
  if (k === 'enter' && url) {
    window.open(url, '_blank', 'noopener')
    trackOpen(a.id)
  } else router.push(detailsTo(a))
}

function onSearchKey(k: SearchKey, text: string) {
  if (k === 'enter' || k === 'shift-enter') {
    const ready = loaded.value && !loading.value && settled === text && q.value === text
    if (ready) {
      const a = shown.value[active.value]
      if (a) openApp(k, a)
      return true
    }
    dropPending()
    let win: Window | null = null
    if (k === 'enter') {
      win = window.open('', '_blank')
      if (win) win.opener = null
    }
    pending = { key: k, text, win }
    // nothing in flight for this text, e.g. the last search failed
    if (q.value === text && !loading.value) search()
    return true
  }
  const a = shown.value[active.value]
  if (!q.value || !a) return false
  const i = k === 'down' ? Math.min(active.value + 1, shown.value.length - 1) : Math.max(active.value - 1, 0)
  active.value = i
  document.getElementById(rowId(shown.value[i]!))?.scrollIntoView({ block: 'nearest' })
  return true
}
onMounted(() => setSearchKeyHandler(onSearchKey))
onBeforeUnmount(() => {
  setSearchKeyHandler(null)
  activeOption.value = ''
})

// the whole row opens the app page, the links and buttons in it do their own thing
function onRow(e: MouseEvent, a: App) {
  if ((e.target as HTMLElement).closest('a, button') || window.getSelection()?.toString()) return
  router.push(detailsTo(a))
}

const suggestions = computed(() => (q.value && !found.value.length ? similarApps(all.value, q.value) : []))
const elsewhere = computed(() => q.value && !shown.value.length && found.value.length > 0)
const mailto = computed(() => `mailto:${adminEmail.value}?subject=${encodeURIComponent('Access to apps in the portal')}`)
</script>

<template>
  <div class="min-h-dvh">
    <SiteHeader search />
    <main class="mx-auto max-w-[1280px] px-4 pt-7 pb-12 sm:px-8 xl:px-10">
      <div
        v-if="error"
        role="alert"
        class="mb-6 flex flex-wrap items-center gap-x-3.5 gap-y-2 rounded-xl border border-warn-line bg-warn-soft px-5 py-4 text-warn"
      >
        <Icon name="info" :size="20" />
        <div class="flex min-w-0 flex-[1_1_280px] flex-col gap-0.5">
          <span class="text-[15px] font-semibold">Couldn't load the apps</span>
          <span class="text-[13.5px]">
            {{ error }}<template v-if="!loaded && cachedFavs.length"> Your favorites below are from your last visit.</template>
          </span>
        </div>
        <button type="button" class="btn btn-secondary" @click="load">Try again</button>
      </div>

      <!-- server down: favorites from the last visit -->
      <section v-if="!loaded && !loading && cachedFavs.length" aria-labelledby="cached-h" class="flex flex-col gap-3">
        <h1 id="cached-h" class="m-0 text-lg font-semibold">Favorites</h1>
        <div class="flex flex-wrap gap-2">
          <a
            v-for="app in cachedFavs"
            :key="app.id"
            :href="openURL(app) || undefined"
            target="_blank"
            rel="noopener"
            class="flex h-10 items-center gap-2 rounded-[10px] border border-line bg-white pr-3 pl-1.5 text-sm font-medium text-ink no-underline hover:border-field hover:text-ink"
          >
            <AppIcon :app="app" :size="28" />
            {{ app.name }}
            <Icon name="external" :size="14" class="text-faint" />
          </a>
        </div>
      </section>

      <!-- first load -->
      <div v-else-if="!loaded && loading" aria-busy="true" aria-label="Loading apps" class="grid gap-8 lg:grid-cols-[200px_1fr]">
        <div class="hidden flex-col gap-2 lg:flex">
          <span v-for="i in 6" :key="i" class="h-8 animate-pulse rounded-lg bg-[#e9ebee]" />
        </div>
        <div class="flex flex-col gap-4">
          <span class="h-8 w-48 animate-pulse rounded-lg bg-[#e9ebee]" />
          <div class="card overflow-hidden">
            <div v-for="i in 8" :key="i" class="flex items-center gap-3 border-t border-line-soft px-4 py-3 first:border-t-0">
              <span class="size-8 animate-pulse rounded-lg bg-[#e9ebee]" />
              <span class="flex flex-col gap-1.5">
                <span class="h-3.5 w-28 animate-pulse rounded bg-[#e9ebee]" />
                <span class="h-2.5 w-48 animate-pulse rounded bg-[#e9ebee]" />
              </span>
            </div>
          </div>
        </div>
      </div>

      <!-- nothing shared with this person at all -->
      <div
        v-else-if="loaded && !all.length"
        class="flex flex-col items-center gap-3.5 rounded-2xl border border-line bg-white px-6 py-12 text-center"
      >
        <span class="flex size-14 items-center justify-center rounded-[14px] bg-chip text-muted"><Icon name="grid" :size="26" /></span>
        <h1 class="m-0 text-xl font-semibold">{{ isAdmin ? 'No apps yet' : 'Nothing is shared with you yet' }}</h1>
        <p class="m-0 max-w-[520px] text-[15px] leading-relaxed text-muted">
          <template v-if="isAdmin">Add the first app and pick who can see it.</template>
          <template v-else-if="myGroups.length">Your groups don't have any apps yet, and none are shared with everyone.</template>
          <template v-else>You aren't in any group, and no app is shared with everyone. A portal admin can add you to a group.</template>
        </p>
        <RouterLink v-if="isAdmin" to="/admin/apps/new" class="btn btn-primary">Add app</RouterLink>
        <template v-else-if="adminEmail">
          <a :href="mailto" class="btn btn-primary">Email portal admins</a>
          <span class="font-mono text-[13px] text-subtle">{{ adminEmail }}</span>
        </template>
      </div>

      <div v-else-if="loaded" class="grid items-start gap-6 lg:grid-cols-[200px_1fr] lg:gap-8">
        <nav aria-label="App lists" class="-mx-4 overflow-x-auto px-4 lg:sticky lg:top-6 lg:mx-0 lg:overflow-visible lg:px-0">
          <ul class="m-0 flex list-none gap-1 p-0 lg:flex-col lg:gap-0.5">
            <li v-for="n in nav" :key="n.key" class="shrink-0">
              <button
                type="button"
                :aria-current="n.active ? 'page' : undefined"
                class="nav-item"
                :class="{ 'nav-item-active': n.active }"
                @click="n.go"
              >
                <span class="flex-1">{{ n.label }}</span>
                <span class="nav-count">{{ n.count }}</span>
              </button>
            </li>
            <template v-if="recentApps.length">
              <li class="hidden px-2.5 pt-4 pb-1.5 lg:block">
                <h2 class="eyebrow m-0">Recently opened</h2>
              </li>
              <li v-for="a in recentApps" :key="`recent-${a.id}`" class="hidden lg:block">
                <component
                  :is="openURL(a) ? 'a' : RouterLink"
                  :href="openURL(a) || undefined"
                  :to="openURL(a) ? undefined : detailsTo(a)"
                  :target="openURL(a) ? '_blank' : undefined"
                  :rel="openURL(a) ? 'noopener' : undefined"
                  class="nav-item no-underline hover:text-ink"
                  @click="openURL(a) && trackOpen(a.id)"
                >
                  <AppIcon :app="a" :size="20" />
                  <span class="min-w-0 flex-1 truncate">{{ a.name }}</span>
                  <template v-if="openURL(a)">
                    <Icon name="external" :size="12" class="shrink-0 text-faint" />
                    <span class="sr-only">(opens in a new tab)</span>
                  </template>
                </component>
              </li>
            </template>
            <li v-if="catNav.length" class="hidden px-2.5 pt-4 pb-1.5 lg:block">
              <h2 class="eyebrow m-0">Categories</h2>
            </li>
            <li v-for="c in catNav" :key="c.key" class="shrink-0">
              <button
                type="button"
                :aria-current="c.active ? 'page' : undefined"
                class="nav-item"
                :class="{ 'nav-item-active': c.active }"
                @click="c.go"
              >
                <span aria-hidden="true" class="size-2 shrink-0 rounded-[2px]" :style="{ background: c.color }" />
                <span class="flex-1">{{ c.label }}</span>
                <span class="nav-count">{{ c.count }}</span>
              </button>
            </li>
          </ul>
        </nav>

        <section aria-labelledby="list-h" class="flex min-w-0 flex-col gap-4">
          <p
            v-if="checksDown"
            role="status"
            class="m-0 flex items-start gap-2.5 rounded-[10px] border border-[#ecd9a8] bg-[#fbf2dc] px-4 py-3 text-[13.5px] leading-normal text-[#4d3a0c]"
          >
            <span aria-hidden="true" class="mt-[7px] size-1.5 shrink-0 rounded-full bg-[#a8740a]" />
            <span>
              <strong class="font-semibold">Health checks are failing for every app.</strong>
              <template v-if="checksDown.dns"> The portal can't resolve internal DNS, so every check fails.</template>
              <template v-else> The problem is most likely on the portal side.</template>
              Your apps are most likely fine.
            </span>
          </p>

          <div class="flex flex-wrap items-center justify-between gap-3">
            <h1 id="list-h" class="m-0 flex items-baseline gap-2.5 text-2xl font-semibold">
              {{ title }}
              <span class="text-sm font-normal text-subtle">{{ countText }}</span>
            </h1>
            <div class="flex flex-wrap items-center gap-2">
              <div role="group" aria-label="Sort" class="flex rounded-lg border border-line bg-white p-px">
                <button
                  v-for="o in ['name', 'popular'] as const"
                  :key="o"
                  type="button"
                  :aria-pressed="sort === o"
                  class="h-8 cursor-pointer rounded-[7px] px-3.5 text-[13px] font-semibold capitalize text-ink aria-pressed:bg-[#e9e7e1]"
                  @click="setSort(o)"
                >
                  {{ o }}
                </button>
              </div>
              <div role="group" aria-label="Layout" class="flex rounded-lg border border-line bg-white p-px">
                <button
                  v-for="l in ['list', 'cards'] as const"
                  :key="l"
                  type="button"
                  :aria-pressed="layout === l"
                  class="h-8 cursor-pointer rounded-[7px] px-3.5 text-[13px] font-semibold capitalize text-ink aria-pressed:bg-[#e9e7e1]"
                  @click="setLayout(l)"
                >
                  {{ l }}
                </button>
              </div>
            </div>
          </div>

          <!-- empty list -->
          <div
            v-if="!loading && !shown.length"
            class="flex flex-col items-center gap-3 rounded-xl border border-line bg-white px-6 py-10 text-center"
          >
            <template v-if="q">
              <p class="m-0 text-lg font-semibold">
                No apps match "{{ q }}"<template v-if="view !== 'all' || category"> in {{ title }}</template>
              </p>
              <p class="m-0 text-sm text-muted">
                <template v-if="elsewhere">There are matches in other lists.</template>
                <template v-else>Check the spelling or try fewer words.</template>
                <template v-if="!isAdmin"> If the app isn't here, it may not be shared with your groups yet.</template>
              </p>
              <div class="flex flex-wrap justify-center gap-2.5">
                <button v-if="elsewhere" type="button" class="btn btn-primary" @click="pick('all')">Search all apps</button>
                <button type="button" class="btn btn-secondary" @click="setQuery({ q: undefined })">Clear search</button>
              </div>
              <div v-if="suggestions.length" class="flex flex-col items-center gap-2 pt-2">
                <span class="text-xs font-semibold text-subtle">Did you mean</span>
                <div class="flex flex-wrap justify-center gap-2">
                  <RouterLink
                    v-for="a in suggestions"
                    :key="a.id"
                    :to="detailsTo(a)"
                    class="flex h-10 items-center gap-2 rounded-[10px] border border-line bg-white pr-3 pl-1.5 text-sm font-medium text-ink no-underline hover:border-field hover:text-ink"
                  >
                    <AppIcon :app="a" :size="28" />
                    {{ a.name }}
                  </RouterLink>
                </div>
              </div>
            </template>
            <template v-else-if="view === 'favorites'">
              <p class="m-0 text-lg font-semibold">No favorites yet</p>
              <p class="m-0 text-sm text-muted">Star an app to keep it here.</p>
            </template>
            <template v-else-if="view === 'start'">
              <p class="m-0 text-lg font-semibold">Nothing under Start here yet</p>
              <p class="m-0 text-sm text-muted">
                <template v-if="isAdmin">Edit an app and tick Start here to list it for new people.</template>
                <template v-else>Portal admins pick the apps for this list.</template>
              </p>
            </template>
            <template v-else>
              <p class="m-0 text-lg font-semibold">No apps in {{ title }}</p>
            </template>
          </div>

          <div v-else-if="layout === 'list'" class="card overflow-x-auto transition-opacity" :class="{ 'opacity-60': loading }">
            <table id="app-results" class="w-full border-collapse text-left text-sm">
              <thead class="bg-panel">
                <tr class="text-[11.5px] tracking-[0.06em] text-subtle uppercase">
                  <th scope="col" class="w-full px-4 py-2.5 font-semibold lg:w-[44%]">App</th>
                  <th scope="col" class="hidden px-4 py-2.5 font-semibold md:table-cell">Category</th>
                  <th scope="col" class="hidden px-4 py-2.5 font-semibold lg:table-cell">Owner</th>
                  <th scope="col" class="hidden px-4 py-2.5 font-semibold lg:table-cell">Version</th>
                  <th scope="col" class="w-px px-4 py-2.5"><span class="sr-only">Open</span></th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="(app, i) in shown"
                  :id="rowId(app)"
                  :key="app.id"
                  class="group cursor-pointer border-t border-line-soft hover:bg-hover"
                  :class="{ 'bg-hover': i === active }"
                  @click="onRow($event, app)"
                >
                  <!-- max-w-0 lets the name and description truncate -->
                  <td class="max-w-0 px-4 py-2.5" :class="{ 'shadow-[inset_3px_0_0_var(--color-accent)]': i === active }">
                    <div class="flex items-center gap-3">
                      <AppIcon :app="app" :size="30" />
                      <div class="flex min-w-0 flex-col">
                        <span class="flex items-center gap-1.5">
                          <RouterLink :to="detailsTo(app)" class="truncate font-semibold text-ink no-underline hover:text-accent-ink">
                            {{ app.name }}
                          </RouterLink>
                          <span
                            v-if="app.visibility === 'groups'"
                            class="shrink-0 rounded border border-line px-1.5 py-px text-[11px] text-subtle"
                            :title="app.groups.length ? `Visible to ${app.groups.join(', ')}` : 'Only admins can see it'"
                            >Restricted</span
                          >
                          <button
                            type="button"
                            :aria-pressed="app.favorite"
                            :aria-label="app.favorite ? `Remove ${app.name} from favorites` : `Add ${app.name} to favorites`"
                            :disabled="favBusy.has(app.id)"
                            class="flex size-5 shrink-0 cursor-pointer items-center justify-center rounded"
                            :class="
                              app.favorite
                                ? 'text-star'
                                : 'text-faint opacity-0 group-hover:opacity-100 hover:text-star focus-visible:opacity-100'
                            "
                            @click="toggleFav(app)"
                          >
                            <Icon name="star" :size="13" :filled="app.favorite" />
                          </button>
                        </span>
                        <span v-if="app.description" class="truncate text-[13px] text-subtle">{{ app.description }}</span>
                      </div>
                    </div>
                  </td>
                  <td class="hidden px-4 py-2.5 whitespace-nowrap md:table-cell">{{ app.category ? catLabel(app.category) : '' }}</td>
                  <td class="hidden px-4 py-2.5 lg:table-cell">
                    <RouterLink
                      v-if="app.owner"
                      :to="`/teams/${encodeURIComponent(app.owner)}`"
                      class="text-ink no-underline hover:text-accent-ink hover:underline"
                      >{{ app.owner }}</RouterLink
                    >
                  </td>
                  <td class="hidden px-4 py-2.5 font-mono text-[13px] whitespace-nowrap text-muted lg:table-cell">{{ app.version }}</td>
                  <td class="px-4 py-2.5 text-right">
                    <a
                      v-if="openURL(app)"
                      :href="openURL(app)"
                      target="_blank"
                      rel="noopener"
                      class="inline-flex h-8 items-center gap-1 rounded-md border border-line bg-white px-2.5 text-[13px] font-semibold text-accent no-underline hover:border-accent hover:text-accent-ink"
                      @click="trackOpen(app.id)"
                    >
                      Open
                      <Icon name="external" :size="12" />
                      <span class="sr-only">{{ app.name }} (opens in a new tab)</span>
                    </a>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div
            v-else
            id="app-results"
            class="grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-4 transition-opacity"
            :class="{ 'opacity-60': loading }"
          >
            <AppCard
              v-for="(app, i) in shown"
              :id="rowId(app)"
              :key="app.id"
              :app="app"
              :active="i === active"
              :envs="envs"
              :category="catLabel(app.category)"
              :runs-on="runsLabel(app.runs_on)"
              :admin="isAdmin"
              :fav-busy="favBusy.has(app.id)"
              hide-status
              @favorite="toggleFav(app)"
              @copy="(url) => copyURL(app, url)"
              @delete="deleting = app"
            />
          </div>
        </section>
      </div>
    </main>

    <p
      v-if="notice"
      role="status"
      class="fixed bottom-5 left-1/2 z-50 m-0 -translate-x-1/2 rounded-lg bg-ink px-4 py-2.5 text-sm font-medium text-white shadow-lg"
    >
      {{ notice }}
    </p>
    <DeleteAppModal v-if="deleting" :app="deleting" @close="deleting = null" @deleted="onDeleted" />
  </div>
</template>

<style scoped>
@reference '../style.css';

.nav-item {
  @apply flex h-9 w-full cursor-pointer items-center gap-2.5 rounded-lg px-2.5 text-left text-sm whitespace-nowrap text-ink hover:bg-[#ecebe6];
}
.nav-item-active {
  @apply bg-[#e9e7e1] font-semibold hover:bg-[#e9e7e1];
}
.nav-count {
  @apply font-mono text-xs font-normal text-subtle;
}
</style>
