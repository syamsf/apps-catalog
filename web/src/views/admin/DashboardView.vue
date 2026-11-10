<script setup lang="ts">
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'
import Avatar from '@/components/Avatar.vue'
import Icon from '@/components/Icon.vue'
import Select from '@/components/Select.vue'
import { type AccessRequest, type App, type Attention, type Dashboard, type MissedSearch, api, errorText, plural, timeAgo } from '@/api'

const data = ref<Dashboard>()
const apps = ref<App[]>([])
const error = ref('')
const requests = ref<AccessRequest[]>([])
const missed = ref<MissedSearch[]>([])
const rowError = ref<Record<string, string>>({})
const picking = ref<number | null>(null)
const pick = ref('')

async function load() {
  error.value = ''
  try {
    const [d, a, r] = await Promise.all([api.dashboard(), api.apps(), api.accessRequests()])
    data.value = d
    apps.value = a.apps
    missed.value = d.missed ?? []
    requests.value = r.requests
  } catch (e) {
    error.value = errorText(e, 'Could not load the dashboard.')
  }
}
load()

const checked = computed(() => apps.value.filter((a) => a.status))
const down = computed(() => checked.value.filter((a) => !a.status!.up))
const emptyGroups = computed(() => data.value?.attention.filter((a) => a.kind === 'group_no_members').length ?? 0)

const stats = computed(() => {
  const d = data.value
  if (!d) return []
  const regular = Math.max(0, d.users - d.admins)
  return [
    { label: 'Apps', value: String(d.apps), note: `${d.apps_everyone} shared with everyone`, to: '/admin/apps' },
    {
      label: 'Groups',
      value: String(d.groups),
      note: emptyGroups.value ? `${plural(emptyGroups.value, 'group')} with no members` : 'Each group gives access to its apps',
      to: '/admin/groups',
    },
    { label: 'Users', value: String(d.users), note: `${plural(d.admins, 'admin')}, ${plural(regular, 'user')}`, to: '/admin/users' },
  ]
})

const usage = computed(() => {
  const u = data.value?.usage
  if (!u) return []
  return [
    { label: 'People', value: u.people },
    { label: 'Opens', value: u.opens },
    { label: 'Searches', value: u.searches },
    { label: 'Searches with no results', value: u.missed },
  ]
})

interface Item {
  key: string
  level: 'down' | 'warn' | 'info'
  title: string
  detail: string
  to: string
}

function link(a: Attention) {
  const t = encodeURIComponent(a.target)
  switch (a.kind) {
    case 'user_no_group':
      return `/admin/users?edit=${t}`
    case 'group_no_members':
      return `/admin/groups/${t}`
    default:
      return `/admin/apps/${t}/edit`
  }
}

const items = computed<Item[]>(() => [
  ...down.value.map((a) => ({
    key: `down-${a.id}`,
    level: 'down' as const,
    title: `${a.name} is down`,
    detail: `${a.status!.error || `HTTP ${a.status!.http_status}`}. Down since ${timeAgo(a.status!.since).toLowerCase()}.`,
    to: `/apps/${encodeURIComponent(a.id)}`,
  })),
  ...(data.value?.attention ?? []).map((a, i) => ({
    key: `${a.kind}-${a.target}-${i}`,
    level: a.kind === 'group_no_members' ? ('info' as const) : ('warn' as const),
    title: a.title,
    detail: a.detail,
    to: link(a),
  })),
])
function groupsOf(appId: string) {
  return apps.value.find((a) => a.id === appId)?.groups ?? []
}

function openPicker(r: AccessRequest) {
  rowError.value = {}
  if (picking.value === r.id) {
    picking.value = null
    return
  }
  picking.value = r.id
  pick.value = groupsOf(r.app_id)[0] ?? ''
}

const busy = ref('')

async function dismiss(m: MissedSearch) {
  const key = `m-${m.query}`
  busy.value = key
  rowError.value = {}
  try {
    await api.dismissSearch(m.query)
    missed.value = missed.value.filter((x) => x.query !== m.query)
  } catch (e) {
    rowError.value = { [key]: errorText(e, 'Could not dismiss this search.') }
  } finally {
    busy.value = ''
  }
}

async function decide(r: AccessRequest, approve: boolean) {
  const key = `r-${r.id}`
  busy.value = key
  rowError.value = {}
  try {
    if (approve) await api.approveAccess(r.id, pick.value)
    else await api.declineAccess(r.id)
    requests.value = requests.value.filter((x) => x.id !== r.id)
    picking.value = null
  } catch (e) {
    rowError.value = { [key]: errorText(e, 'Could not save this decision.') }
  } finally {
    busy.value = ''
  }
}

const dot = { down: 'bg-down', warn: 'bg-star', info: 'bg-disabled' }
</script>

<template>
  <div class="flex flex-wrap items-center justify-between gap-x-6 gap-y-3">
    <h1 class="m-0 text-[28px] font-semibold">Dashboard</h1>
    <RouterLink to="/admin/apps/new" class="btn btn-primary px-[18px]">Add app</RouterLink>
  </div>

  <div
    v-if="error"
    role="alert"
    class="flex flex-wrap items-center gap-x-3.5 gap-y-2 rounded-xl border border-warn-line bg-warn-soft px-5 py-4 text-warn"
  >
    <Icon name="info" :size="20" />
    <span class="flex-[1_1_240px] text-[15px] font-semibold">{{ error }}</span>
    <button type="button" class="btn btn-secondary" @click="load">Try again</button>
  </div>

  <div v-else-if="!data" aria-busy="true" aria-label="Loading dashboard" class="grid grid-cols-2 gap-4 lg:grid-cols-4">
    <div v-for="i in 4" :key="i" class="flex h-[112px] flex-col gap-3 rounded-xl border border-line bg-white p-5">
      <span class="h-3 w-16 animate-pulse rounded bg-[#e9ebee]" />
      <span class="h-7 w-12 animate-pulse rounded bg-[#e9ebee]" />
      <span class="h-3 w-32 animate-pulse rounded bg-[#e9ebee]" />
    </div>
  </div>

  <template v-if="data">
    <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
      <RouterLink
        v-for="s in stats"
        :key="s.label"
        :to="s.to"
        class="card flex flex-col gap-1.5 px-5 py-[18px] text-ink no-underline hover:border-field hover:text-ink"
      >
        <span class="text-[13px] text-subtle">{{ s.label }}</span>
        <span class="text-[30px] leading-[1.1] font-semibold">{{ s.value }}</span>
        <span class="text-[13px] text-muted">{{ s.note }}</span>
      </RouterLink>
      <RouterLink
        to="/admin/apps?status=down"
        class="flex flex-col gap-1.5 rounded-xl border bg-white px-5 py-[18px] text-ink no-underline hover:text-ink"
        :class="down.length ? 'border-warn-line hover:border-warn' : 'border-line hover:border-field'"
      >
        <span class="text-[13px] text-subtle">Health</span>
        <span class="text-[30px] leading-[1.1] font-semibold">
          <template v-if="checked.length">{{ checked.length - down.length }} / {{ checked.length }} up</template>
          <template v-else>-</template>
        </span>
        <span v-if="down.length" class="inline-flex items-center gap-1.5 text-[13px] font-medium text-down">
          <span class="size-2 rounded-full bg-down" />
          {{ down.length === 1 ? `${down[0]!.name} is down` : `${down.length} apps are down` }}
        </span>
        <span v-else class="text-[13px] text-muted">{{ checked.length ? 'Everything answers' : 'No health checks yet' }}</span>
      </RouterLink>
    </div>

    <section aria-labelledby="use-h" class="card flex min-w-0 flex-col gap-3 px-6 pt-5 pb-5">
      <h2 id="use-h" class="section-title">Last 7 days</h2>
      <dl class="m-0 grid grid-cols-2 gap-4 lg:grid-cols-4">
        <div v-for="u in usage" :key="u.label" class="flex flex-col gap-1">
          <dt class="text-[13px] text-subtle">{{ u.label }}</dt>
          <dd class="m-0 text-[26px] leading-[1.1] font-semibold">{{ u.value }}</dd>
        </div>
      </dl>
    </section>

    <section v-if="requests.length" aria-labelledby="req-h" class="card flex min-w-0 flex-col gap-2 px-6 pt-5 pb-5">
      <h2 id="req-h" class="section-title mb-1 flex items-center gap-2">
        Access requests
        <span class="rounded-full bg-chip px-2 py-px text-xs font-semibold text-muted">{{ requests.length }}</span>
      </h2>
      <div v-for="r in requests" :key="r.id" class="flex flex-col gap-2 rounded-lg border border-line-soft px-4 py-3">
        <div class="flex flex-wrap items-center gap-x-3 gap-y-2">
          <span class="flex min-w-0 flex-[1_1_260px] flex-col gap-[3px]">
            <span class="text-sm font-semibold">{{ r.person }} wants {{ r.app_name }}</span>
            <span v-if="r.note" class="text-[13px] leading-normal text-subtle">{{ r.note }}</span>
          </span>
          <span class="shrink-0 text-xs text-faint" :title="new Date(r.created_at).toLocaleString()">{{ timeAgo(r.created_at) }}</span>
          <button type="button" class="btn btn-secondary" :disabled="busy === `r-${r.id}`" @click="openPicker(r)">Approve</button>
          <button type="button" class="btn btn-secondary" :disabled="busy === `r-${r.id}`" @click="decide(r, false)">Decline</button>
        </div>
        <div v-if="picking === r.id" class="flex flex-wrap items-center gap-x-3 gap-y-2">
          <template v-if="groupsOf(r.app_id).length">
            <label :for="`pick-${r.id}`" class="text-[13px] text-subtle">Add to group</label>
            <Select
              :id="`pick-${r.id}`"
              v-model="pick"
              :options="groupsOf(r.app_id).map((g) => ({ value: g, label: g }))"
              class="w-56 max-w-full"
            />
            <button type="button" class="btn btn-primary" :disabled="!pick || busy === `r-${r.id}`" @click="decide(r, true)">
              Approve
            </button>
          </template>
          <span v-else class="text-[13px] text-subtle">
            Share {{ r.app_name }} with a group first.
            <RouterLink :to="`/admin/apps/${encodeURIComponent(r.app_id)}/edit`">Edit {{ r.app_name }}</RouterLink>
          </span>
        </div>
        <p v-if="rowError[`r-${r.id}`]" role="alert" class="m-0 text-[13px] text-warn">{{ rowError[`r-${r.id}`] }}</p>
      </div>
    </section>

    <section aria-labelledby="miss-h" class="card flex min-w-0 flex-col gap-2 px-6 pt-5 pb-5">
      <h2 id="miss-h" class="section-title mb-1">Searches with no results</h2>
      <p v-if="!missed.length" class="m-0 py-2 text-sm text-subtle">No failed searches in the last 30 days.</p>
      <div v-for="m in missed" :key="m.query" class="flex flex-col gap-1 rounded-lg border border-line-soft px-4 py-3">
        <div class="flex flex-wrap items-center gap-x-3 gap-y-2">
          <span class="min-w-0 flex-[1_1_200px] break-words text-sm font-semibold">{{ m.query }}</span>
          <span class="text-[13px] text-subtle">{{ plural(m.people, 'person', 'people') }}</span>
          <span class="text-xs text-faint" :title="new Date(m.last_at).toLocaleString()">searched {{ timeAgo(m.last_at).toLowerCase() }}</span>
          <RouterLink v-if="m.hidden" :to="`/admin/apps/${encodeURIComponent(m.hidden.id)}/edit`" class="text-[13px]">
            Exists, but they can't see it
          </RouterLink>
          <button type="button" class="btn btn-secondary" :disabled="busy === `m-${m.query}`" @click="dismiss(m)">Dismiss</button>
        </div>
        <p v-if="rowError[`m-${m.query}`]" role="alert" class="m-0 text-[13px] text-warn">{{ rowError[`m-${m.query}`] }}</p>
      </div>
    </section>

    <div class="grid items-start gap-5 xl:grid-cols-2">
      <section aria-labelledby="att-h" class="card flex min-w-0 flex-col gap-2 px-6 pt-5 pb-6">
        <h2 id="att-h" class="section-title mb-1 flex items-center gap-2">
          Needs a look
          <span v-if="items.length" class="rounded-full bg-chip px-2 py-px text-xs font-semibold text-muted">{{ items.length }}</span>
        </h2>
        <p v-if="!items.length" class="m-0 py-2 text-sm text-subtle">Nothing needs a look right now.</p>
        <RouterLink
          v-for="it in items"
          :key="it.key"
          :to="it.to"
          class="flex items-start gap-3 rounded-lg border px-4 py-3 no-underline"
          :class="it.level === 'down' ? 'border-[#f3d9c9] bg-[#fdf3ec] hover:bg-warn-soft' : 'border-line-soft hover:bg-hover'"
        >
          <span class="mt-[7px] size-2 shrink-0 rounded-full" :class="dot[it.level]" />
          <span class="flex min-w-0 flex-1 flex-col gap-[3px]">
            <span class="text-sm font-semibold" :class="it.level === 'down' ? 'text-down' : 'text-ink'">{{ it.title }}</span>
            <span class="text-[13px] leading-normal text-subtle">{{ it.detail }}</span>
          </span>
          <Icon name="arrow" :size="16" class="mt-0.5 text-faint" />
        </RouterLink>
      </section>

      <section aria-labelledby="act-h" class="card flex min-w-0 flex-col px-6 pt-5 pb-3">
        <h2 id="act-h" class="section-title mb-2">Recent changes</h2>
        <p v-if="!data.changes.length" class="m-0 py-2 text-sm text-subtle">No changes yet.</p>
        <ul class="m-0 list-none p-0">
          <li v-for="c in data.changes" :key="c.id" class="flex items-center gap-2.5 py-2">
            <Avatar :name="c.actor" :size="28" />
            <span class="min-w-0 flex-1 text-[13.5px] leading-snug">
              <span class="font-semibold">{{ c.actor }}</span> {{ c.summary }}
            </span>
            <span class="shrink-0 text-xs text-faint" :title="new Date(c.at).toLocaleString()">{{ timeAgo(c.at) }}</span>
          </li>
        </ul>
      </section>
    </div>
  </template>
</template>
