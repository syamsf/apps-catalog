<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import AppIcon from '@/components/AppIcon.vue'
import Avatar from '@/components/Avatar.vue'
import Icon from '@/components/Icon.vue'
import SaveBar from '@/components/SaveBar.vue'
import { type App, ApiError, type Group, ID_PATTERN, type User, type UserRef, api, errorText, plural } from '@/api'
import { flash, setTitle } from '@/session'

const props = defineProps<{ name?: string }>()
const router = useRouter()
const isEdit = computed(() => !!props.name)

const groupName = ref('')
const description = ref('')
const members = ref<UserRef[]>([])
const selected = ref<string[]>([])
const original = ref<Group>()

const users = ref<User[]>([])
const apps = ref<App[]>([])
const loading = ref(true)
const loadError = ref('')
const notFound = ref(false)

const personQ = ref('')
const personOpen = ref(false)
const personActive = ref(0)
const appQuery = ref('')
const saving = ref(false)
const saved = ref('')
const errors = ref<Record<string, string>>({})
const generalError = ref('')
const confirming = ref(false)
const deleteBusy = ref(false)
const deleteError = ref('')
const top = ref<HTMLElement>()

async function load() {
  loading.value = true
  loadError.value = ''
  notFound.value = false
  errors.value = {}
  generalError.value = ''
  confirming.value = false
  try {
    const [u, a, g] = await Promise.all([
      api.users(),
      api.apps(),
      props.name ? api.group(props.name) : Promise.resolve(undefined),
    ])
    users.value = u.users
    apps.value = a.apps
    original.value = g
    groupName.value = g?.name ?? ''
    description.value = g?.description ?? ''
    members.value = g ? [...g.members] : []
    selected.value = g ? g.apps.map((x) => x.id) : []
    setTitle(g ? `Edit ${g.name}` : 'New group')
    snapshot.value = state()
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) notFound.value = true
    else loadError.value = errorText(e, 'Could not load the group.')
  } finally {
    loading.value = false
  }
}
watch(() => props.name, load, { immediate: true })

const memberSubjects = computed(() => new Set(members.value.map((m) => m.subject)))
const candidates = computed(() => users.value.filter((u) => !memberSubjects.value.has(u.subject)))
const personMatches = computed(() => {
  const q = personQ.value.trim().toLowerCase()
  if (!q) return []
  return candidates.value
    .filter((u) => [u.name, u.email, u.username].some((v) => v?.toLowerCase().includes(q)))
    .slice(0, 6)
})
watch(personQ, () => {
  personActive.value = 0
  personOpen.value = !!personQ.value.trim()
})

function addMember(u: User | undefined) {
  if (!u) return
  members.value = [...members.value, { subject: u.subject, name: u.name, email: u.email }].sort((a, b) =>
    a.name.localeCompare(b.name),
  )
  personQ.value = ''
  saved.value = ''
  delete errors.value.members
}
function onPersonKey(e: KeyboardEvent) {
  const n = personMatches.value.length
  if (e.key === 'ArrowDown' && n) {
    e.preventDefault()
    personOpen.value = true
    personActive.value = (personActive.value + 1) % n
  } else if (e.key === 'ArrowUp' && n) {
    e.preventDefault()
    personActive.value = (personActive.value - 1 + n) % n
  } else if (e.key === 'Enter') {
    e.preventDefault()
    if (personOpen.value) addMember(personMatches.value[personActive.value])
  } else if (e.key === 'Escape') {
    personOpen.value = false
  }
}
const otherGroups = (u: User) => (u.groups.length ? `in ${u.groups.join(', ')}` : 'no group yet')
function removeMember(subject: string) {
  members.value = members.value.filter((m) => m.subject !== subject)
  saved.value = ''
}

function onNameInput() {
  delete errors.value.name
  saved.value = ''
}

const shownApps = computed(() => {
  const q = appQuery.value.trim().toLowerCase()
  return apps.value.filter((a) => !q || a.name.toLowerCase().includes(q) || a.id.includes(q))
})
function toggleApp(id: string) {
  selected.value = selected.value.includes(id) ? selected.value.filter((x) => x !== id) : [...selected.value, id]
  saved.value = ''
  delete errors.value.apps
}

const summaryItems = computed(() => {
  const items = Object.values(errors.value)
  if (generalError.value) items.unshift(generalError.value)
  return items
})

async function save() {
  if (saving.value) return
  saved.value = ''
  generalError.value = ''
  errors.value = {}
  const name = groupName.value.trim()
  if (!ID_PATTERN.test(name)) errors.value.name = 'Use only lowercase letters, numbers, and dashes for the group name.'
  if (Object.keys(errors.value).length) {
    await nextTick()
    top.value?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    return
  }
  saving.value = true
  const input = {
    name,
    description: description.value.trim(),
    members: members.value.map((m) => m.subject),
    apps: selected.value,
  }
  try {
    const g = isEdit.value ? await api.updateGroup(props.name!, input) : await api.createGroup(input)
    if (!isEdit.value) {
      snapshot.value = ''
      flash.value = { text: `${g.name} was created.` }
      router.push('/admin/groups')
      return
    }
    original.value = g
    members.value = [...g.members]
    selected.value = g.apps.map((x) => x.id)
    groupName.value = g.name
    description.value = g.description
    snapshot.value = state()
    saved.value = `${g.name} was saved. ${plural(g.members.length, 'person', 'people')} now see ${plural(g.apps.length, 'app')} through this group.`
    if (g.name !== props.name) router.replace(`/admin/groups/${encodeURIComponent(g.name)}`)
  } catch (e) {
    if (e instanceof ApiError && Object.keys(e.fields).length) errors.value = { ...e.fields }
    else generalError.value = errorText(e, 'Could not save the group. Try again.')
    await nextTick()
    top.value?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  } finally {
    saving.value = false
  }
}

function cancelDelete() {
  confirming.value = false
  deleteError.value = ''
}

async function doDelete() {
  if (!props.name || deleteBusy.value) return
  deleteBusy.value = true
  deleteError.value = ''
  try {
    await api.deleteGroup(props.name)
    flash.value = { text: `${props.name} was deleted.` }
    router.push('/admin/groups')
  } catch (e) {
    deleteError.value = errorText(e, 'Could not delete the group. Try again.')
  } finally {
    deleteBusy.value = false
  }
}

// unsaved changes, said in words for the save bar
const state = () =>
  JSON.stringify({
    name: groupName.value.trim(),
    description: description.value.trim(),
    members: members.value.map((m) => m.subject).sort(),
    apps: [...selected.value].sort(),
  })
const snapshot = ref('')
const changes = computed(() => {
  if (!snapshot.value) return []
  const was = JSON.parse(snapshot.value) as { name: string; description: string; members: string[]; apps: string[] }
  const out: string[] = []
  if (was.name !== groupName.value.trim()) out.push('name')
  if (was.description !== description.value.trim()) out.push('description')
  const who = (id: string) => users.value.find((u) => u.subject === id)?.name ?? original.value?.members.find((m) => m.subject === id)?.name ?? id
  const what = (id: string) => apps.value.find((a) => a.id === id)?.name ?? id
  const now = new Set(members.value.map((m) => m.subject))
  const add = [...now].filter((x) => !was.members.includes(x)).map(who)
  const del = was.members.filter((x) => !now.has(x)).map(who)
  if (add.length || del.length) out.push(`members: ${[add.length ? `added ${add.join(', ')}` : '', del.length ? `removed ${del.join(', ')}` : ''].filter(Boolean).join(', ')}`)
  const addA = selected.value.filter((x) => !was.apps.includes(x)).map(what)
  const delA = was.apps.filter((x) => !selected.value.includes(x)).map(what)
  if (addA.length || delA.length) out.push(`apps: ${[addA.length ? `added ${addA.join(', ')}` : '', delA.length ? `removed ${delA.join(', ')}` : ''].filter(Boolean).join(', ')}`)
  return out
})
const changeText = computed(() => {
  const t = changes.value.join('; ')
  return t ? t[0]!.toUpperCase() + t.slice(1) : ''
})
</script>

<template>
  <div class="flex flex-col gap-5">
    <nav aria-label="Breadcrumb" class="flex items-center gap-1.5 text-sm text-subtle">
      <RouterLink to="/admin/groups" class="font-medium text-accent no-underline hover:text-accent-ink">Groups</RouterLink>
      <Icon name="back" :size="14" class="rotate-180 text-faint" />
      <span :class="{ 'font-mono': isEdit }">{{ isEdit ? name : 'New group' }}</span>
    </nav>
    <h1 class="m-0 text-[28px] font-semibold">{{ isEdit ? `Edit ${name}` : 'New group' }}</h1>

    <div v-if="loading" aria-busy="true" aria-label="Loading the group" class="flex flex-col gap-4">
      <div class="h-28 animate-pulse rounded-xl bg-[#e9ebee]" />
      <div class="grid gap-5 lg:grid-cols-2">
        <div class="h-72 animate-pulse rounded-xl bg-[#e9ebee]" />
        <div class="h-72 animate-pulse rounded-xl bg-[#e9ebee]" />
      </div>
    </div>
    <div v-else-if="notFound" class="card flex max-w-[640px] flex-col gap-3 p-6">
      <p class="m-0">There is no group named <span class="font-mono">{{ name }}</span>.</p>
      <div><RouterLink to="/admin/groups" class="btn btn-secondary">Back to groups</RouterLink></div>
    </div>
    <div
      v-else-if="loadError"
      role="alert"
      class="flex flex-wrap items-center gap-x-3.5 gap-y-2 rounded-xl border border-warn-line bg-warn-soft px-5 py-4 text-warn"
    >
      <Icon name="info" :size="20" />
      <span class="flex-[1_1_240px] text-[15px] font-semibold">{{ loadError }}</span>
      <button type="button" class="btn btn-secondary" @click="load">Try again</button>
    </div>

    <template v-else>
      <div v-if="saved || summaryItems.length" ref="top" class="flex flex-col gap-3">
        <p v-if="saved" role="status" class="status-ok">{{ saved }}</p>
        <div v-if="summaryItems.length" role="alert" class="rounded-[10px] bg-warn-soft px-4 py-3.5 text-sm text-warn">
          <p class="m-0 mb-1.5 font-semibold">Fix these before saving:</p>
          <ul class="m-0 flex list-disc flex-col gap-1 pl-5">
            <li v-for="(err, i) in summaryItems" :key="i">{{ err }}</li>
          </ul>
        </div>
      </div>

      <form id="group-form" novalidate class="flex flex-col gap-5" @submit.prevent="save">
        <section aria-label="Basics" class="card grid gap-4 p-6 sm:grid-cols-[minmax(220px,320px)_1fr]">
          <div class="flex flex-col gap-1.5">
            <label for="g-name" class="label">Group name</label>
            <input
              id="g-name"
              v-model="groupName"
              type="text"
              spellcheck="false"
              autocomplete="off"
              placeholder="data-team"
              class="field font-mono"
              :aria-invalid="!!errors.name"
              @input="onNameInput"
            />
            <span v-if="errors.name" class="field-error">{{ errors.name }}</span>
            <span v-else class="hint">Lowercase letters, numbers and dashes.</span>
          </div>
          <div class="flex flex-col gap-1.5">
            <label for="g-desc" class="label">Description</label>
            <input
              id="g-desc"
              v-model="description"
              type="text"
              placeholder="Data engineering and analytics"
              class="field"
              @input="saved = ''"
            />
          </div>
        </section>

        <div class="flex flex-col gap-5 lg:flex-row lg:items-start">
          <section aria-labelledby="g-members" class="card flex min-w-0 flex-1 flex-col gap-3 p-5">
            <div class="flex items-baseline justify-between gap-3">
              <h2 id="g-members" class="section-title text-[17px]">Members</h2>
              <span class="text-[13px] text-subtle">{{ plural(members.length, 'person', 'people') }}</span>
            </div>
            <div class="relative">
              <label for="g-add" class="sr-only">Add a person</label>
              <Icon name="search" :size="16" class="pointer-events-none absolute top-3 left-3 text-subtle" />
              <input
                id="g-add"
                v-model="personQ"
                type="search"
                role="combobox"
                autocomplete="off"
                aria-autocomplete="list"
                aria-controls="g-add-list"
                :aria-expanded="personOpen && !!personMatches.length"
                :aria-activedescendant="personOpen && personMatches.length ? `g-add-${personActive}` : undefined"
                :placeholder="candidates.length ? 'Add a person by name or email' : 'Everyone is already in this group'"
                :disabled="!candidates.length"
                class="field h-10 pl-9"
                @keydown="onPersonKey"
                @focus="personOpen = !!personQ.trim()"
                @blur="personOpen = false"
              />
              <ul
                v-show="personOpen"
                id="g-add-list"
                role="listbox"
                class="absolute top-[calc(100%+4px)] right-0 left-0 z-30 m-0 list-none rounded-[10px] border border-line bg-white p-1.5 shadow-[0_8px_24px_rgba(21,23,28,0.12)]"
              >
                <li
                  v-for="(u, i) in personMatches"
                  :id="`g-add-${i}`"
                  :key="u.subject"
                  role="option"
                  :aria-selected="i === personActive"
                  class="flex cursor-pointer items-center gap-2.5 rounded-md px-2 py-1.5"
                  :class="i === personActive ? 'bg-accent-soft' : ''"
                  @pointerenter="personActive = i"
                  @pointerdown.prevent
                  @click="addMember(u)"
                >
                  <Avatar :name="u.name || u.username" :size="28" />
                  <span class="flex min-w-0 flex-1 flex-col">
                    <span class="truncate text-sm font-medium">{{ u.name || u.username }}</span>
                    <span class="truncate text-xs text-subtle">{{ u.email || u.username }}</span>
                  </span>
                  <span class="shrink-0 text-xs text-faint">{{ otherGroups(u) }}</span>
                </li>
                <li v-if="!personMatches.length" class="px-2 py-1.5 text-sm text-subtle">Nobody matches "{{ personQ.trim() }}".</li>
                <li v-else class="px-2 pt-1 text-[11.5px] text-faint">Enter adds the highlighted person</li>
              </ul>
            </div>
            <span v-if="errors.members" class="field-error">{{ errors.members }}</span>
            <ul class="m-0 list-none p-0">
              <li v-for="m in members" :key="m.subject" class="flex items-center gap-3 border-t border-line-soft py-2 first:border-t-0">
                <Avatar :name="m.name" :size="32" />
                <div class="flex min-w-0 flex-1 flex-col">
                  <span class="truncate text-sm font-medium">{{ m.name }}</span>
                  <span class="truncate text-[12.5px] text-subtle">{{ m.email }}</span>
                </div>
                <button type="button" :aria-label="`Remove ${m.name} from this group`" class="icon-btn" @click="removeMember(m.subject)">
                  <Icon name="x" :size="14" />
                </button>
              </li>
            </ul>
            <p v-if="!members.length" class="m-0 text-sm text-subtle">No members yet. Search above to add people.</p>
          </section>

          <section aria-labelledby="g-apps" class="card flex min-w-0 flex-1 flex-col gap-3 p-5">
            <div class="flex items-baseline justify-between gap-3">
              <h2 id="g-apps" class="section-title text-[17px]">Apps this group can see</h2>
              <span class="text-[13px] whitespace-nowrap text-subtle">{{ selected.length }} selected</span>
            </div>
            <div class="relative">
              <Icon name="search" :size="16" class="pointer-events-none absolute top-3 left-3 text-subtle" />
              <input v-model="appQuery" type="search" aria-label="Filter apps" placeholder="Filter apps" class="field h-10 pl-9" />
            </div>
            <span v-if="errors.apps" class="field-error">{{ errors.apps }}</span>
            <div>
              <label
                v-for="a in shownApps"
                :key="a.id"
                class="flex min-h-11 cursor-pointer items-center gap-3 border-t border-line-soft py-1 text-sm first:border-t-0"
              >
                <input type="checkbox" class="check" :checked="selected.includes(a.id)" @change="toggleApp(a.id)" />
                <AppIcon :app="a" :size="28" />
                <span class="min-w-0 flex-1 truncate" :class="{ 'text-subtle': a.visibility === 'everyone' }">{{ a.name }}</span>
                <span v-if="a.visibility === 'everyone'" class="chip-ok font-normal">Everyone already sees it</span>
                <span v-else-if="!a.groups.length" class="text-[12.5px] text-faint">Admins only</span>
                <span v-else-if="a.groups.some((g) => g !== name)" class="truncate text-[12.5px] text-faint">
                  Also: {{ a.groups.filter((g) => g !== name).join(', ') }}
                </span>
              </label>
              <p v-if="!shownApps.length" class="m-0 pt-1 text-sm text-subtle">
                {{ apps.length ? 'No apps match.' : 'No apps yet.' }}
              </p>
            </div>
          </section>
        </div>
      </form>

      <section
        v-if="isEdit && original"
        aria-labelledby="g-danger"
        class="flex flex-col gap-3 rounded-xl border border-warn-line bg-white p-5"
      >
        <div class="flex flex-wrap items-center gap-x-6 gap-y-3">
          <div class="flex flex-[1_1_320px] flex-col gap-1">
            <h2 id="g-danger" class="m-0 text-base font-semibold text-warn">Delete this group</h2>
            <p class="m-0 text-sm leading-normal text-muted">
              {{ plural(original.members.length, 'person', 'people') }} lose the apps they only had through {{ original.name }}. The apps and
              people stay in the portal.
            </p>
          </div>
          <button v-if="!confirming" type="button" class="btn btn-danger-outline px-[18px] font-semibold" @click="confirming = true">
            Delete group
          </button>
        </div>
        <div
          v-if="confirming"
          role="alertdialog"
          aria-labelledby="g-del-q"
          class="flex flex-wrap items-center gap-x-4 gap-y-2.5 rounded-[10px] bg-warn-soft px-4 py-3.5"
        >
          <span id="g-del-q" class="flex-[1_1_260px] text-sm font-medium text-warn-ink">Delete {{ original.name }}? This can't be undone.</span>
          <button type="button" class="btn btn-secondary" @click="cancelDelete">Keep it</button>
          <button type="button" class="btn btn-danger" :disabled="deleteBusy" @click="doDelete">
            {{ deleteBusy ? 'Deleting...' : 'Yes, delete' }}
          </button>
        </div>
        <p v-if="deleteError" role="alert" class="status-error">{{ deleteError }}</p>
      </section>
    </template>

    <SaveBar
      :show="!loading && !notFound && !loadError && (!isEdit || changes.length > 0)"
      :label="isEdit ? 'Unsaved changes' : 'New group'"
      :detail="isEdit ? changeText : 'Not created yet'"
      :saving="saving"
      :action="isEdit ? 'Save group' : 'Create group'"
      form="group-form"
      @cancel="router.push('/admin/groups')"
    />
  </div>
</template>
