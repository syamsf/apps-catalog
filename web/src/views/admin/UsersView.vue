<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import Avatar from '@/components/Avatar.vue'
import Icon from '@/components/Icon.vue'
import MenuSelect from '@/components/MenuSelect.vue'
import PasswordInput from '@/components/PasswordInput.vue'
import SidePanel from '@/components/SidePanel.vue'
import { ApiError, type Group, type User, api, daysAgo, errorText, plural } from '@/api'
import { localLoginOn, me, settings } from '@/session'

const route = useRoute()
const router = useRouter()

const users = ref<User[]>([])
const groups = ref<Group[]>([])
const groupNames = computed(() => groups.value.map((g) => g.name))
const loading = ref(true)
const error = ref('')
const query = ref('')
// the person shown in the side panel, also in the url so links can open it
const editing = ref(String(route.query.edit ?? ''))
watch(editing, (v) => router.replace({ query: { ...route.query, edit: v || undefined } }))
const current = computed(() => users.value.find((u) => u.subject === editing.value))
const saved = ref('')
const saveError = ref('')
const busy = ref(new Set<string>())
const mySubject = computed(() => me.value?.user.subject ?? '')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [u, g, a] = await Promise.all([api.users(), api.groups(), api.apps()])
    users.value = u.users
    everyoneCount.value = a.apps.filter((x) => x.visibility === 'everyone').length
    groups.value = g.groups
    if (current.value) openUser(current.value)
  } catch (e) {
    error.value = errorText(e, 'Could not load users.')
  } finally {
    loading.value = false
  }
}
load()

const role = ref('all')
const source = ref('all')
const noGroup = ref(false)
const lonely = (u: User) => !u.groups.length && !u.is_admin
const lonelyCount = computed(() => users.value.filter(lonely).length)
const rows = computed(() => {
  const q = query.value.trim().toLowerCase()
  return users.value.filter(
    (u) =>
      (!q || u.name.toLowerCase().includes(q) || u.email.toLowerCase().includes(q) || u.username.toLowerCase().includes(q)) &&
      (role.value === 'all' || (role.value === 'admin') === u.is_admin) &&
      (source.value === 'all' || u.source === source.value) &&
      (!noGroup.value || lonely(u)),
  )
})
const filtered = computed(() => !!query.value.trim() || role.value !== 'all' || source.value !== 'all' || noGroup.value)
function clearFilters() {
  query.value = ''
  role.value = 'all'
  source.value = 'all'
  noGroup.value = false
}
const groupApps = (name: string) => groups.value.find((g) => g.name === name)?.apps.map((a) => a.name) ?? []
const everyoneCount = ref(0)

const display = (u: User) => u.name || u.username
// no double period after names like "Rani T."
const end = (s: string) => (s.endsWith('.') ? s : `${s}.`)

function clearStatus() {
  saved.value = ''
  saveError.value = ''
}

function openUser(u: User) {
  adding.value = false
  editing.value = u.subject
  clearStatus()
  acctErrors.value = {}
  Object.assign(acct, { name: u.name, email: u.email, is_admin: u.is_admin, password: '', confirmRemove: false })
}
function closePanel() {
  editing.value = ''
  adding.value = false
}

async function toggleGroup(u: User, g: string) {
  if (busy.value.has(u.subject)) return
  const had = u.groups.includes(g)
  const next = had ? u.groups.filter((x) => x !== g) : [...u.groups, g]
  const prev = u.groups
  busy.value.add(u.subject)
  clearStatus()
  u.groups = next
  try {
    const updated = await api.setUserGroups(u.subject, next)
    u.groups = updated.groups
    saved.value = had ? `Removed ${display(u)} from ${g}.` : `Added ${display(u)} to ${g}.`
  } catch (e) {
    u.groups = prev
    saveError.value = errorText(e, 'Could not save the groups. Try again.')
  } finally {
    busy.value.delete(u.subject)
  }
}

// account panel
const acct = reactive({ name: '', email: '', is_admin: false, password: '', confirmRemove: false })
const acctErrors = ref<Record<string, string>>({})


async function run(u: User, fn: () => Promise<void>, fail: string) {
  busy.value.add(u.subject)
  clearStatus()
  acctErrors.value = {}
  try {
    await fn()
  } catch (e) {
    if (e instanceof ApiError && Object.keys(e.fields).length) acctErrors.value = e.fields
    else saveError.value = errorText(e, fail)
  } finally {
    busy.value.delete(u.subject)
  }
}

function saveAccount(u: User) {
  return run(
    u,
    async () => {
      const updated = await api.updateUser(u.subject, { name: acct.name, email: acct.email, is_admin: acct.is_admin })
      Object.assign(u, updated)
      saved.value = end(`Saved ${display(u)}`)
    },
    'Could not save. Try again.',
  )
}

function setPassword(u: User) {
  if (acct.password.length < 8) {
    acctErrors.value = { password: 'Use at least 8 characters' }
    return
  }
  return run(
    u,
    async () => {
      await api.setUserPassword(u.subject, acct.password)
      acct.password = ''
      saved.value = `${end(`New password set for ${display(u)}`)} Tell them in person or over a private channel.`
    },
    'Could not set the password. Try again.',
  )
}

function removeUser(u: User) {
  return run(
    u,
    async () => {
      await api.deleteUser(u.subject)
      users.value = users.value.filter((x) => x.subject !== u.subject)
      editing.value = ''
      saved.value = end(`Removed ${display(u)}`)
    },
    'Could not remove. Try again.',
  )
}

// add a local user
const adding = ref(false)
const add = reactive({ username: '', name: '', email: '', password: '', is_admin: false, groups: [] as string[] })
const addErrors = ref<Record<string, string>>({})
const addBusy = ref(false)

function openAdd() {
  editing.value = ''
  adding.value = true
  Object.assign(add, { username: '', name: '', email: '', password: '', is_admin: false, groups: [] })
  addErrors.value = {}
  clearStatus()
}

async function createUser() {
  const e: Record<string, string> = {}
  if (!/^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$/.test(add.username.trim())) e.username = 'Use letters, numbers and single dashes'
  if (add.password.length < 8) e.password = 'Use at least 8 characters'
  addErrors.value = e
  if (Object.keys(e).length) return
  addBusy.value = true
  clearStatus()
  try {
    const u = await api.createUser({ ...add, username: add.username.trim() })
    users.value = [...users.value, u].sort((a, b) => display(a).localeCompare(display(b)))
    adding.value = false
    saved.value = `${end(`Added ${display(u)}`)} They can sign in with the username ${u.username}.`
  } catch (err) {
    if (err instanceof ApiError && Object.keys(err.fields).length) addErrors.value = err.fields
    else saveError.value = errorText(err, 'Could not add the user. Try again.')
  } finally {
    addBusy.value = false
  }
}

const toggleAddGroup = (g: string) =>
  (add.groups = add.groups.includes(g) ? add.groups.filter((x) => x !== g) : [...add.groups, g])

const cols = 'grid-cols-[minmax(240px,2fr)_minmax(200px,2fr)_90px_120px_20px]'
</script>

<template>
  <div class="flex flex-wrap items-end justify-between gap-4">
    <div class="flex flex-col gap-1">
      <h1 class="m-0 text-[28px] font-semibold">Users</h1>
      <p class="m-0 text-sm text-subtle">Put people in groups to give them apps.</p>
    </div>
    <button v-if="localLoginOn" type="button" class="btn btn-primary px-[18px]" @click="openAdd">Add person</button>
  </div>

  <p v-if="saved && !current && !adding" role="status" class="status-ok">{{ saved }}</p>
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
    <div class="relative max-w-[300px] flex-[1_1_220px]">
      <Icon name="search" :size="16" class="pointer-events-none absolute top-3 left-3 text-subtle" />
      <input v-model="query" type="search" aria-label="Search people" placeholder="Name or email" class="field h-10 pl-9" />
    </div>
    <MenuSelect
      v-model="role"
      label="Role"
      :options="[
        { value: 'all', label: 'Any' },
        { value: 'admin', label: 'Admin' },
        { value: 'user', label: 'User' },
      ]"
    />
    <MenuSelect
      v-model="source"
      label="Sign-in"
      :options="[
        { value: 'all', label: 'Any' },
        { value: 'sso', label: 'SSO' },
        { value: 'local', label: 'Portal account' },
      ]"
    />
    <button
      v-if="lonelyCount"
      type="button"
      :aria-pressed="noGroup"
      class="inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-full border px-3 text-[13px] font-medium"
      :class="noGroup ? 'border-warn bg-warn text-white' : 'border-warn-line bg-warn-soft text-warn'"
      @click="noGroup = !noGroup"
    >
      No group <span class="font-semibold">{{ lonelyCount }}</span>
    </button>
    <span class="ml-auto text-[13px] text-subtle">
      {{ filtered ? `${rows.length} of ${users.length}` : users.length }} {{ users.length === 1 ? 'person' : 'people' }}
    </span>
  </div>

  <div class="card overflow-x-auto">
    <div class="min-w-[820px]">
      <div class="grid gap-5 border-b border-line px-5 py-2.5 text-xs font-semibold tracking-[0.04em] text-subtle uppercase" :class="cols">
        <span>Person</span><span>Groups</span><span>Role</span><span>Last sign-in</span><span class="sr-only">Open</span>
      </div>
      <template v-if="loading && !users.length">
        <div v-for="i in 5" :key="i" class="grid items-center gap-5 border-b border-line-soft px-5 py-3 last:border-b-0" :class="cols">
          <span class="flex items-center gap-3">
            <span class="size-[34px] animate-pulse rounded-full bg-[#e9ebee]" />
            <span class="h-3.5 w-32 animate-pulse rounded bg-[#e9ebee]" />
          </span>
          <span class="h-5 w-16 animate-pulse rounded bg-[#e9ebee]" />
          <span class="h-3 w-10 animate-pulse rounded bg-[#e9ebee]" />
          <span class="h-3 w-16 animate-pulse rounded bg-[#e9ebee]" />
          <span />
        </div>
      </template>
      <ul class="m-0 list-none p-0">
        <li
          v-for="u in rows"
          :key="u.subject"
          class="relative grid items-center gap-5 border-b border-line-soft px-5 py-3 text-sm last:border-b-0"
          :class="[cols, editing === u.subject ? 'bg-accent-soft' : 'hover:bg-hover']"
        >
          <div class="flex min-w-0 items-center gap-3">
            <Avatar :name="display(u)" :size="34" />
            <div class="flex min-w-0 flex-col">
              <span class="flex min-w-0 items-center gap-2">
                <button
                  type="button"
                  class="cursor-pointer truncate text-left font-semibold after:absolute after:inset-0 after:content-['']"
                  @click="openUser(u)"
                >
                  {{ display(u) }}
                </button>
                <span
                  class="shrink-0 rounded px-1.5 py-px text-[11px] font-medium"
                  :class="u.source === 'local' ? 'bg-accent-soft text-accent-ink' : 'bg-chip text-subtle'"
                >
                  {{ u.source === 'local' ? 'Portal account' : 'SSO' }}
                </span>
              </span>
              <span class="truncate text-[12.5px] text-subtle">{{ u.email || (u.source === 'local' ? u.username : '') }}</span>
            </div>
          </div>
          <div class="flex flex-wrap gap-1">
            <span v-for="g in u.groups" :key="g" class="chip-group">{{ g }}</span>
            <span v-if="lonely(u)" class="chip-warn">No group</span>
            <span v-else-if="!u.groups.length" class="text-[13px] text-subtle">Sees every app</span>
          </div>
          <span>
            <span v-if="u.is_admin" class="rounded-md bg-ink px-2 py-0.5 text-xs font-semibold text-white">Admin</span>
            <span v-else class="text-[13px] text-muted">User</span>
          </span>
          <span class="text-[13px] text-muted" :title="u.last_login ? new Date(u.last_login).toLocaleString() : undefined">
            {{ daysAgo(u.last_login) }}
          </span>
          <Icon name="arrow" :size="16" class="text-faint" />
        </li>
      </ul>
      <div v-if="!loading && !rows.length" class="flex flex-col items-center gap-3 px-5 py-10 text-center">
        <template v-if="users.length">
          <p class="m-0 text-base font-semibold">No one matches</p>
          <button type="button" class="btn btn-secondary" @click="clearFilters">Clear search and filters</button>
        </template>
        <template v-else>
          <p class="m-0 text-base font-semibold">No one has signed in yet</p>
          <p class="m-0 text-sm text-subtle">People show up here after their first sign-in.</p>
        </template>
      </div>
    </div>
  </div>

  <SidePanel v-if="current" :title="display(current)" @close="closePanel">
    <template #head>
      <Avatar :name="display(current)" :size="44" />
      <div class="flex min-w-0 flex-col gap-0.5">
        <span class="flex min-w-0 items-center gap-2">
          <h2 id="panel-title" class="m-0 truncate text-[17px] font-semibold">{{ display(current) }}</h2>
          <span
            class="shrink-0 rounded px-1.5 py-px text-[11px] font-medium"
            :class="current.source === 'local' ? 'bg-accent-soft text-accent-ink' : 'bg-chip text-subtle'"
          >
            {{ current.source === 'local' ? 'Portal account' : 'SSO' }}
          </span>
        </span>
        <span class="truncate text-[12.5px] text-subtle">
          {{ current.email || current.username }}, last sign-in {{ daysAgo(current.last_login).toLowerCase() }}
        </span>
      </div>
    </template>

    <p v-if="saved" role="status" class="status-ok">{{ saved }}</p>
    <p v-if="saveError" role="alert" class="status-error">{{ saveError }}</p>
    <p v-if="lonely(current)" class="m-0 flex items-start gap-2 rounded-lg bg-warn-soft px-3 py-2.5 text-[13px] leading-normal text-warn">
      <Icon name="info" :size="16" class="mt-px shrink-0" />
      Not in any group. {{ display(current) }} only sees the {{ plural(everyoneCount, 'app') }} shared with everyone.
    </p>

    <fieldset class="m-0 flex flex-col gap-2 border-0 p-0" :disabled="busy.has(current.subject)">
      <legend class="mb-2 p-0 text-sm font-semibold">Groups</legend>
      <div v-if="groupNames.length" class="rounded-[10px] border border-line px-3.5">
        <label
          v-for="g in groupNames"
          :key="g"
          class="flex min-h-11 cursor-pointer items-center gap-3 border-b border-line-soft last:border-b-0"
        >
          <input type="checkbox" class="check" :checked="current.groups.includes(g)" @change="toggleGroup(current, g)" />
          <span class="min-w-0 flex-1 font-mono text-[13.5px]">{{ g }}</span>
          <span class="truncate text-[12.5px] text-faint">{{ groupApps(g).join(', ') || 'no apps' }}</span>
        </label>
      </div>
      <p v-else class="m-0 text-sm text-subtle">
        No groups yet. <RouterLink to="/admin/groups/new" class="link">Create one first</RouterLink>.
      </p>
      <span class="hint">Saved as you tick. {{ display(current) }} sees new apps on the next page load.</span>
    </fieldset>

    <template v-if="current.source === 'local'">
      <form class="flex flex-col gap-3" novalidate @submit.prevent="saveAccount(current)">
        <h3 class="m-0 text-sm font-semibold">Account</h3>
        <div class="grid gap-3 sm:grid-cols-2">
          <div class="flex flex-col gap-1.5">
            <label for="a-name" class="label">Name</label>
            <input id="a-name" v-model="acct.name" type="text" class="field" />
          </div>
          <div class="flex flex-col gap-1.5">
            <label for="a-email" class="label">Email</label>
            <input id="a-email" v-model="acct.email" type="email" class="field" :aria-invalid="!!acctErrors.email" />
            <span v-if="acctErrors.email" class="field-error">{{ acctErrors.email }}</span>
          </div>
        </div>
        <label class="flex cursor-pointer items-start gap-3 rounded-[10px] border border-line px-3.5 py-3">
          <input v-model="acct.is_admin" type="checkbox" class="check mt-px" :disabled="current.subject === mySubject" />
          <span class="flex flex-col gap-0.5">
            <span class="text-sm font-medium">Portal admin</span>
            <span class="text-[12.5px] text-subtle">
              {{ current.subject === mySubject ? "You can't remove your own admin role." : 'Can open the admin area and see every app.' }}
            </span>
          </span>
        </label>
        <div><button type="submit" class="btn btn-secondary btn-sm">Save account</button></div>
      </form>
      <form class="flex flex-col gap-1.5" novalidate @submit.prevent="setPassword(current)">
        <label for="a-pass" class="label">New password</label>
        <div class="flex gap-2">
          <div class="min-w-0 flex-1">
            <PasswordInput id="a-pass" v-model="acct.password" autocomplete="new-password" :invalid="!!acctErrors.password" />
          </div>
          <button type="submit" class="btn btn-secondary">Set password</button>
        </div>
        <span v-if="acctErrors.password" class="field-error">{{ acctErrors.password }}</span>
        <span v-else class="hint">At least 8 characters. Tell them in person or over a private channel.</span>
      </form>
    </template>
    <div v-else class="flex flex-col gap-1.5">
      <h3 class="m-0 text-sm font-semibold">Role</h3>
      <p class="m-0 text-sm">{{ current.is_admin ? 'Portal admin' : 'User' }}</p>
      <p class="m-0 text-[12.5px] leading-normal text-subtle">
        Name, email and password come from SSO.
        <template v-if="settings?.admin_group">
          The admin role comes from the LLDAP group <span class="chip-group">{{ settings.admin_group }}</span>. Change it in LLDAP.
        </template>
      </p>
    </div>

    <template v-if="current.subject !== mySubject" #footer>
      <template v-if="!acct.confirmRemove">
        <button type="button" class="btn btn-danger-outline btn-sm" @click="acct.confirmRemove = true">Remove person</button>
        <span class="min-w-0 flex-1 text-xs leading-normal text-subtle">
          <template v-if="current.source === 'local'">They can't sign in anymore.</template>
          <template v-else>They come back on their next SSO sign-in, without groups.</template>
        </span>
      </template>
      <template v-else>
        <span class="min-w-0 flex-1 text-sm font-medium">Remove {{ display(current) }} and sign them out?</span>
        <button type="button" class="btn btn-secondary btn-sm" @click="acct.confirmRemove = false">Keep</button>
        <button type="button" class="btn btn-danger btn-sm" @click="removeUser(current)">Yes, remove</button>
      </template>
    </template>
  </SidePanel>

  <SidePanel v-if="adding" title="Add person" @close="closePanel">
    <template #head>
      <div class="flex flex-col gap-0.5">
        <h2 id="panel-title" class="m-0 text-[17px] font-semibold">Add person</h2>
        <span class="text-[12.5px] text-subtle">A portal account, for people who can't use company SSO.</span>
      </div>
    </template>
    <form id="add-person" class="flex flex-col gap-4" novalidate @submit.prevent="createUser">
      <p v-if="saveError" role="alert" class="status-error">{{ saveError }}</p>
      <div class="flex flex-col gap-1.5">
        <label for="n-user" class="label">Username</label>
        <input
          id="n-user"
          v-model="add.username"
          type="text"
          autocomplete="off"
          autocapitalize="none"
          spellcheck="false"
          class="field font-mono"
          :aria-invalid="!!addErrors.username"
        />
        <span v-if="addErrors.username" class="field-error">{{ addErrors.username }}</span>
        <span v-else class="hint">What they type to sign in. Can't be changed later.</span>
      </div>
      <div class="flex flex-col gap-1.5">
        <label for="n-pass" class="label">Password</label>
        <PasswordInput id="n-pass" v-model="add.password" autocomplete="new-password" :invalid="!!addErrors.password" />
        <span v-if="addErrors.password" class="field-error">{{ addErrors.password }}</span>
        <span v-else class="inline-flex items-center gap-1.5 text-[12.5px]" :class="add.password.length >= 8 ? 'font-medium text-ok' : 'text-subtle'">
          <Icon name="check" :size="14" :class="add.password.length >= 8 ? '' : 'text-faint'" />
          At least 8 characters. They can change it after signing in.
        </span>
      </div>
      <div class="grid gap-3 sm:grid-cols-2">
        <div class="flex flex-col gap-1.5">
          <label for="n-name" class="label">Name <span class="font-normal text-subtle">(optional)</span></label>
          <input id="n-name" v-model="add.name" type="text" autocomplete="off" class="field" />
        </div>
        <div class="flex flex-col gap-1.5">
          <label for="n-email" class="label">Email <span class="font-normal text-subtle">(optional)</span></label>
          <input id="n-email" v-model="add.email" type="email" autocomplete="off" class="field" :aria-invalid="!!addErrors.email" />
          <span v-if="addErrors.email" class="field-error">{{ addErrors.email }}</span>
        </div>
      </div>
      <fieldset v-if="groupNames.length" class="m-0 flex flex-col gap-2 border-0 p-0">
        <legend class="label mb-2 p-0">Groups</legend>
        <div class="flex flex-wrap gap-2">
          <button
            v-for="g in groupNames"
            :key="g"
            type="button"
            :aria-pressed="add.groups.includes(g)"
            class="inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-full border px-3 font-mono text-[13px]"
            :class="add.groups.includes(g) ? 'border-accent bg-accent-soft text-accent-ink' : 'border-field bg-white text-ink hover:border-[#aeb3ba]'"
            @click="toggleAddGroup(g)"
          >
            <Icon v-if="add.groups.includes(g)" name="check" :size="14" />
            {{ g }}
          </button>
        </div>
      </fieldset>
      <label class="flex cursor-pointer items-start gap-3 rounded-[10px] border border-line px-3.5 py-3">
        <input v-model="add.is_admin" type="checkbox" class="check mt-px" />
        <span class="flex flex-col gap-0.5">
          <span class="text-sm font-medium">Portal admin</span>
          <span class="text-[12.5px] text-subtle">Can open the admin area and see every app.</span>
        </span>
      </label>
    </form>
    <template #footer>
      <span class="flex-1" />
      <button type="button" class="btn btn-ghost" @click="closePanel">Cancel</button>
      <button type="submit" form="add-person" class="btn btn-primary px-5" :disabled="addBusy">
        {{ addBusy ? 'Adding...' : 'Add person' }}
      </button>
    </template>
  </SidePanel>
</template>
