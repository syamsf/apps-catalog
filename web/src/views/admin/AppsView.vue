<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import AppIcon from '@/components/AppIcon.vue'
import DeleteAppModal from '@/components/DeleteAppModal.vue'
import Icon from '@/components/Icon.vue'
import MenuSelect from '@/components/MenuSelect.vue'
import Popover from '@/components/Popover.vue'
import { type App, type Group, api, errorText, statusTitle } from '@/api'
import { flash, optionLabel, settings } from '@/session'

const route = useRoute()
const router = useRouter()

const apps = ref<App[]>([])
const groups = ref<Group[]>([])
const loading = ref(true)
const error = ref('')
const toast = ref(flash.value && !flash.value.error ? flash.value.text : '')
flash.value = null

const query = ref('')
const group = ref('all')
const status = ref(['up', 'down', 'none'].includes(String(route.query.status)) ? String(route.query.status) : 'all')
watch(status, (v) => router.replace({ query: { ...route.query, status: v === 'all' ? undefined : v } }))

const groupOptions = computed(() => [
  { value: 'all', label: 'Any group' },
  { value: 'everyone', label: 'Everyone' },
  { value: 'admins', label: 'Admins only' },
  ...groups.value.map((g) => ({ value: g.name, label: g.name })),
])
const statusOptions = [
  { value: 'all', label: 'Any' },
  { value: 'up', label: 'Up' },
  { value: 'down', label: 'Down' },
  { value: 'none', label: 'Not checked' },
]

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [a, g] = await Promise.all([api.apps(), api.groups()])
    apps.value = a.apps
    groups.value = g.groups
  } catch (e) {
    error.value = errorText(e, 'Could not load apps.')
  } finally {
    loading.value = false
  }
}
load()

type SortKey = 'name' | 'owner' | 'status'
const sortKey = ref<SortKey>('name')
const sortDesc = ref(false)
function sortBy(k: SortKey) {
  if (sortKey.value === k) sortDesc.value = !sortDesc.value
  else {
    sortKey.value = k
    sortDesc.value = false
  }
}
const statusRank = (a: App) => (!a.status ? 1 : a.status.up ? 2 : 0)
const statusOf = (a: App) => (!a.status ? 'none' : a.status.up ? 'up' : 'down')

const rows = computed(() => {
  const q = query.value.trim().toLowerCase()
  const list = apps.value.filter((a) => {
    const okQ = !q || [a.name, a.id, a.owner].some((v) => v.toLowerCase().includes(q))
    const okG =
      group.value === 'all' ||
      (group.value === 'everyone'
        ? a.visibility === 'everyone'
        : group.value === 'admins'
          ? a.visibility === 'groups' && !a.groups.length
          : a.visibility === 'groups' && a.groups.includes(group.value))
    const okS = status.value === 'all' || statusOf(a) === status.value
    return okQ && okG && okS
  })
  const dir = sortDesc.value ? -1 : 1
  return list.sort((a, b) => {
    let d = 0
    if (sortKey.value === 'owner') d = a.owner.localeCompare(b.owner)
    else if (sortKey.value === 'status') d = statusRank(a) - statusRank(b)
    return (d || a.name.localeCompare(b.name)) * dir
  })
})
const filtered = computed(() => !!query.value.trim() || group.value !== 'all' || status.value !== 'all')
function clearFilters() {
  query.value = ''
  group.value = 'all'
  status.value = 'all'
}

const deleting = ref<App | null>(null)
function onDeleted() {
  const a = deleting.value
  if (!a) return
  apps.value = apps.value.filter((x) => x.id !== a.id)
  toast.value = `${a.name} was deleted from the catalog.`
  deleting.value = null
}

const cols = 'grid-cols-[minmax(220px,2.2fr)_1fr_1fr_minmax(160px,1.6fr)_0.9fr_minmax(130px,1fr)_32px]'
const headers: { label: string; sort?: SortKey }[] = [
  { label: 'App', sort: 'name' },
  { label: 'Category' },
  { label: 'Runs on' },
  { label: 'Visible to' },
  { label: 'Owner', sort: 'owner' },
  { label: 'Status', sort: 'status' },
]
</script>

<template>
  <div class="flex flex-wrap items-center justify-between gap-x-6 gap-y-3">
    <h1 class="m-0 text-[28px] font-semibold">Apps</h1>
    <div class="flex flex-wrap gap-2.5">
      <RouterLink to="/admin/apps/import" class="btn btn-secondary">
        <Icon name="upload" :size="16" />
        Import
      </RouterLink>
      <RouterLink to="/admin/apps/new" class="btn btn-primary px-[18px]">Add app</RouterLink>
    </div>
  </div>

  <p v-if="toast" role="status" class="status-ok">{{ toast }}</p>
  <div
    v-if="error"
    role="alert"
    class="flex flex-wrap items-center gap-x-3.5 gap-y-2 rounded-xl border border-warn-line bg-warn-soft px-5 py-4 text-warn"
  >
    <Icon name="info" :size="20" />
    <span class="flex-[1_1_240px] text-[15px] font-semibold">{{ error }}</span>
    <button type="button" class="btn btn-secondary" @click="load">Try again</button>
  </div>

  <div class="flex flex-wrap items-center gap-2.5">
    <div class="relative max-w-[340px] flex-[1_1_240px]">
      <Icon name="search" :size="16" class="pointer-events-none absolute top-3 left-3 text-subtle" />
      <input v-model="query" type="search" aria-label="Search apps" placeholder="Search name, ID, or owner" class="field h-10 pl-9" />
    </div>
    <MenuSelect v-model="group" label="Visible to" :options="groupOptions" />
    <MenuSelect v-model="status" label="Status" :options="statusOptions" />
    <span class="ml-auto text-[13px] text-subtle">
      {{ filtered ? `${rows.length} of ${apps.length}` : apps.length }} {{ apps.length === 1 ? 'app' : 'apps' }}
    </span>
  </div>

  <div class="card overflow-x-auto">
    <div class="min-w-[1000px]">
      <div class="grid items-center gap-4 border-b border-line px-5 py-2.5 text-xs font-semibold tracking-[0.04em] text-subtle uppercase" :class="cols">
        <template v-for="h in headers" :key="h.label">
          <button
            v-if="h.sort"
            type="button"
            class="inline-flex cursor-pointer items-center gap-1 justify-self-start uppercase hover:text-ink"
            :class="{ 'text-ink': sortKey === h.sort }"
            :aria-label="`Sort by ${h.label.toLowerCase()}`"
            @click="sortBy(h.sort)"
          >
            {{ h.label }}
            <Icon
              name="chevron"
              :size="12"
              :class="[sortKey === h.sort ? '' : 'text-faint', { 'rotate-180': sortKey === h.sort && sortDesc }]"
            />
          </button>
          <span v-else>{{ h.label }}</span>
        </template>
        <span class="sr-only">Actions</span>
      </div>

      <template v-if="loading && !apps.length">
        <div v-for="i in 5" :key="i" class="grid items-center gap-4 border-b border-line-soft px-5 py-3 last:border-b-0" :class="cols">
          <span class="flex items-center gap-3">
            <span class="size-8 animate-pulse rounded-lg bg-[#e9ebee]" />
            <span class="h-3.5 w-28 animate-pulse rounded bg-[#e9ebee]" />
          </span>
          <span v-for="j in 5" :key="j" class="h-3 w-16 animate-pulse rounded bg-[#e9ebee]" />
        </div>
      </template>

      <ul class="m-0 list-none p-0">
        <li
          v-for="a in rows"
          :key="a.id"
          class="relative grid items-center gap-4 border-b border-line-soft px-5 py-3 text-sm last:border-b-0 hover:bg-hover"
          :class="cols"
        >
          <div class="flex min-w-0 items-center gap-3">
            <AppIcon :app="a" :size="32" />
            <div class="flex min-w-0 flex-col gap-px">
              <RouterLink
                :to="`/admin/apps/${encodeURIComponent(a.id)}/edit`"
                class="truncate font-semibold text-ink no-underline after:absolute after:inset-0 after:content-[''] hover:text-ink"
              >
                {{ a.name }}
              </RouterLink>
              <span class="truncate font-mono text-xs text-subtle">{{ a.id }}</span>
            </div>
          </div>
          <span class="truncate">{{ optionLabel(settings?.categories, a.category) }}</span>
          <span class="truncate">{{ optionLabel(settings?.runs_on, a.runs_on) }}</span>
          <div class="flex flex-wrap gap-1">
            <span v-if="a.visibility === 'everyone'" class="chip-ok">Everyone</span>
            <template v-else>
              <span v-for="g in a.groups" :key="g" class="chip-group">{{ g }}</span>
              <span v-if="!a.groups.length" class="chip-warn">Admins only</span>
            </template>
          </div>
          <span class="truncate">{{ a.owner }}</span>
          <span class="inline-flex items-center gap-1.5 text-[13px]" :title="statusTitle(a.status)">
            <template v-if="!a.status">
              <span class="size-2 rounded-full bg-disabled" /><span class="text-faint">Not checked</span>
            </template>
            <template v-else-if="a.status.up">
              <span class="size-2 rounded-full bg-ok" /><span class="font-medium text-subtle">Up</span>
              <span class="font-mono text-xs text-faint">{{ a.status.latency_ms }} ms</span>
            </template>
            <template v-else>
              <span class="size-2 rounded-full bg-down" /><span class="font-medium text-down">Down</span>
            </template>
          </span>
          <Popover
            :label="`More for ${a.name}`"
            align="right"
            fixed
            panel-class="w-48"
            class="relative z-10 focus-within:z-30"
            button-class="icon-btn aria-expanded:bg-chip aria-expanded:text-ink"
          >
            <template #trigger><Icon name="more" :size="18" /></template>
            <template #default="{ close }">
              <RouterLink :to="`/admin/apps/${encodeURIComponent(a.id)}/edit`" role="menuitem" class="menu-item">
                <Icon name="pencil" :size="16" class="text-subtle" />
                Edit app
              </RouterLink>
              <RouterLink :to="`/apps/${encodeURIComponent(a.id)}`" role="menuitem" class="menu-item">
                <Icon name="external" :size="16" class="text-subtle" />
                View in catalog
              </RouterLink>
              <div class="menu-sep" />
              <button type="button" role="menuitem" class="menu-item text-warn hover:text-warn" @click="(close(false), (deleting = a), (toast = ''))">
                <Icon name="trash" :size="16" />
                Delete
              </button>
            </template>
          </Popover>
        </li>
      </ul>

      <div v-if="!loading && !rows.length" class="flex flex-col items-center gap-3 px-5 py-10 text-center">
        <template v-if="apps.length">
          <p class="m-0 text-base font-semibold">No apps match</p>
          <button type="button" class="btn btn-secondary" @click="clearFilters">Clear search and filters</button>
        </template>
        <template v-else>
          <p class="m-0 text-base font-semibold">No apps yet</p>
          <p class="m-0 text-sm text-subtle">Add the first one and pick who can see it, or import many from a YAML file.</p>
          <div class="flex flex-wrap justify-center gap-2.5">
            <RouterLink to="/admin/apps/new" class="btn btn-primary">Add app</RouterLink>
            <RouterLink to="/admin/apps/import" class="btn btn-secondary">Import from YAML</RouterLink>
          </div>
        </template>
      </div>
    </div>
  </div>

  <DeleteAppModal v-if="deleting" :app="deleting" :groups="groups" @close="deleting = null" @deleted="onDeleted" />
</template>
