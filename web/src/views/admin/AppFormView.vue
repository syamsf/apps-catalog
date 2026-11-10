<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import AppCard from '@/components/AppCard.vue'
import AppIcon from '@/components/AppIcon.vue'
import DeleteAppModal from '@/components/DeleteAppModal.vue'
import IconPicker from '@/components/IconPicker.vue'
import Icon from '@/components/Icon.vue'
import Popover from '@/components/Popover.vue'
import SaveBar from '@/components/SaveBar.vue'
import Select from '@/components/Select.vue'
import SiteHeader from '@/components/SiteHeader.vue'
import {
  type App,
  ApiError,
  type AppInput,
  type DetailsInput,
  type Group,
  ID_PATTERN,
  type Status,
  LINK_KINDS,
  type Visibility,
  api,
  errorText,
  plural,
  slugify,
  statusTitle,
} from '@/api'
import { flash, setTitle, settings } from '@/session'

// ownerMode: the owner team's form, only the details can change and no admin calls are made
const props = defineProps<{ id?: string; ownerMode?: boolean }>()
const router = useRouter()

const isEdit = computed(() => !!props.id)
const MAX_ICON = 256 * 1024

const form = reactive({
  id: '',
  name: '',
  description: '',
  owner: '',
  contact: '',
  category: '',
  runs_on: '',
  version: '',
  start_here: false,
  access: '',
  health_url: '',
  urls: {} as Record<string, string>,
  links: {} as Record<string, string>,
  visibility: 'groups' as Visibility,
  groups: [] as string[],
  icon_color: '',
  icon: '',
})

const original = ref<App>()
const groups = ref<Group[]>([])
const loading = ref(true)
const loadError = ref('')
const notFound = ref(false)
const idTouched = ref(false)
const saving = ref(false)
const errors = ref<Record<string, string>>({})
const generalError = ref('')
const summary = ref<HTMLElement>()
const deleting = ref(false)

const envs = computed(() => settings.value?.environments ?? [])
const categories = computed(() => settings.value?.categories ?? [])
const runsOn = computed(() => settings.value?.runs_on ?? [])
const categoryOptions = computed(() => [
  { value: '', label: 'None' },
  ...categories.value.map((o) => ({ value: o.key, label: o.label })),
])
const runsOnOptions = computed(() => [
  { value: '', label: 'None' },
  ...runsOn.value.map((o) => ({ value: o.key, label: o.label })),
])
const ownerOptions = computed(() => {
  const opts = groups.value.map((g) => ({ value: g.name, label: g.name }))
  if (form.owner && !opts.some((o) => o.value === form.owner)) opts.unshift({ value: form.owner, label: form.owner })
  if (isEdit.value) opts.unshift({ value: '', label: 'No owner team' })
  return opts
})
const palette = computed(() => settings.value?.icon_palette ?? [])
const hasProd = computed(() => envs.value.some((e) => e.key === 'prod'))

// icon state
const pendingFile = ref<File | null>(null)
const previewURL = ref('')
const removeIcon = ref(false)
const iconError = ref('')
const fileInput = ref<HTMLInputElement>()
const showUploaded = computed(() => !!pendingFile.value || (!!original.value?.has_icon && !removeIcon.value))

function setPreview(url: string) {
  if (previewURL.value) URL.revokeObjectURL(previewURL.value)
  previewURL.value = url
}
onBeforeUnmount(() => setPreview(''))

function fillFrom(app: App) {
  Object.assign(form, {
    id: app.id,
    name: app.name,
    description: app.description,
    owner: app.owner,
    contact: app.contact,
    category: app.category,
    runs_on: app.runs_on,
    version: app.version,
    start_here: app.start_here,
    access: app.access,
    health_url: app.health_url,
    urls: { ...app.urls },
    links: { ...app.links },
    visibility: app.visibility,
    groups: [...app.groups],
    icon_color: app.icon_color,
    icon: app.icon,
  })
}

async function load() {
  let leaving = false
  loading.value = true
  loadError.value = ''
  notFound.value = false
  errors.value = {}
  generalError.value = ''
  pendingFile.value = null
  removeIcon.value = false
  setPreview('')
  try {
    const [g, app] = await Promise.all([
      props.ownerMode ? { groups: [] } : api.groups(),
      props.id ? api.app(props.id) : Promise.resolve(undefined),
    ])
    if (props.ownerMode && !app?.can_edit) {
      leaving = true
      router.replace(backTo.value)
      return
    }
    groups.value = g.groups
    original.value = app
    if (app) {
      fillFrom(app)
      idTouched.value = true
      setTitle(`Edit ${app.name}`)
    } else {
      const s = settings.value
      Object.assign(form, {
        id: '',
        name: '',
        description: '',
        owner: '',
        contact: '',
        category: s?.categories[0]?.key ?? '',
        runs_on: s?.runs_on[0]?.key ?? '',
        version: '',
        start_here: false,
        access: '',
        health_url: '',
        urls: {},
        links: {},
        visibility: s?.new_app_visibility ?? 'groups',
        groups: [],
        icon_color: '',
        icon: '',
      })
      idTouched.value = false
      setTitle('Add app')
    }
    rows.value = startRows()
    snapshot.value = state()
    check.value = null
    const fromFlash = flash.value
    if (fromFlash?.error) {
      generalError.value = fromFlash.text
      flash.value = null
    }
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) notFound.value = true
    // no access: the app page explains it
    else if (props.ownerMode && e instanceof ApiError && e.status === 403) {
      leaving = true
      router.replace(backTo.value)
    } else loadError.value = errorText(e, 'Could not load the form.')
  } finally {
    if (!leaving) loading.value = false
  }
}
watch(() => props.id, load, { immediate: true })

function onName() {
  clearErr('name')
  if (!isEdit.value && !idTouched.value) {
    form.id = slugify(form.name)
    clearErr('id')
  }
}
function onId() {
  idTouched.value = true
  clearErr('id')
}
function clearErr(key: string) {
  if (errors.value[key]) {
    const next = { ...errors.value }
    delete next[key]
    errors.value = next
  }
}

function urlPlaceholder(key: string) {
  if (key === 'prod') return 'app.internal.yourcompany.id'
  if (key === 'dev' || key === 'development') return 'app.dev.internal.yourcompany.id'
  return `app.${key}.internal.yourcompany.id`
}
const linkPlaceholder = (key: string) => (key === 'repo' ? 'https://gitea.yourcompany.id/team/app' : 'https://...')

async function onFile() {
  const input = fileInput.value
  const file = input?.files?.[0]
  if (input) input.value = ''
  if (file) iconError.value = await acceptFile(file)
}

// keeps file as the icon to upload on save, or returns what is wrong with it
function acceptFile(file: File): Promise<string> {
  const okType = file.type === 'image/png' || file.type === 'image/svg+xml' || /\.(png|svg)$/i.test(file.name)
  if (!okType) return Promise.resolve('Use a PNG or SVG file.')
  if (file.size > MAX_ICON) return Promise.resolve(`${file.name} is ${Math.ceil(file.size / 1024)} KB. The limit is 256 KB.`)
  const url = URL.createObjectURL(file)
  const accept = () => {
    pendingFile.value = file
    removeIcon.value = false
    form.icon = ''
    iconError.value = ''
    setPreview(url)
    return ''
  }
  if (file.type === 'image/svg+xml' || /\.svg$/i.test(file.name)) return Promise.resolve(accept())
  return new Promise((resolve) => {
    const img = new Image()
    img.onload = () => {
      if (img.naturalWidth < 128 || img.naturalHeight < 128) {
        URL.revokeObjectURL(url)
        resolve(`The image is ${img.naturalWidth} x ${img.naturalHeight} px. Use at least 128 x 128 px.`)
        return
      }
      resolve(accept())
    }
    img.onerror = () => {
      URL.revokeObjectURL(url)
      resolve("This file can't be read as an image.")
    }
    img.src = url
  })
}

const picking = ref(false)
function useTabler(name: string) {
  form.icon = name
  pendingFile.value = null
  setPreview('')
  iconError.value = ''
  // an uploaded icon would hide the Tabler one
  removeIcon.value = !!original.value?.has_icon
}

function useDefaultIcon() {
  form.icon = ''
  pendingFile.value = null
  setPreview('')
  iconError.value = ''
  removeIcon.value = !!original.value?.has_icon
}

const iconPreviewApp = computed(() => ({
  id: form.id || slugify(form.name) || 'app',
  name: form.name,
  icon_color: form.icon_color,
  icon: form.icon,
  has_icon: !!original.value?.has_icon && !removeIcon.value,
  icon_version: original.value?.icon_version ?? 0,
}))

const swatches = computed(() => [
  { key: '', label: 'Pick the color from the app ID', bg: '#F0F1F3', text: 'Auto', fg: '#15171C' },
  ...palette.value.map((c) => ({ key: c, label: `Use color ${c}`, bg: c, text: '', fg: '#FFFFFF' })),
])

function toggleGroup(name: string) {
  form.groups = form.groups.includes(name) ? form.groups.filter((g) => g !== name) : [...form.groups, name]
  clearErr('groups')
}

const visSummary = computed(() => {
  if (form.visibility === 'everyone') return 'Every signed-in user will see this app in the catalog.'
  const picked = groups.value.filter((g) => form.groups.includes(g.name))
  if (!picked.length) return 'Pick at least one group. Until then, nobody but admins can see this app.'
  const people = new Set(picked.flatMap((g) => g.members.map((m) => m.subject))).size
  return `Members of ${picked.map((g) => g.name).join(', ')} (${people} ${people === 1 ? 'person' : 'people'}) will see this app.`
})
const noGroupsWarning = computed(() => form.visibility === 'groups' && form.groups.length === 0)

function validate() {
  const e: Record<string, string> = {}
  if (!form.name.trim()) e.name = 'Add a name.'
  if (!isEdit.value && !ID_PATTERN.test(form.id)) e.id = 'Use only lowercase letters, numbers, and dashes for the app ID.'
  if (hasProd.value) {
    if (!form.urls.prod?.trim()) {
      const label = envs.value.find((x) => x.key === 'prod')?.label ?? 'production'
      e['urls.prod'] = `Add the ${label.toLowerCase()} URL.`
    }
  } else if (envs.value.length && !envs.value.some((x) => form.urls[x.key]?.trim())) {
    e[`urls.${envs.value[0]!.key}`] = 'Add at least one URL.'
  }
  if (!isEdit.value && !form.owner) e.owner = 'Pick the owner team.'
  return e
}

const summaryItems = computed(() => {
  const items = Object.values(errors.value)
  if (generalError.value) items.unshift(generalError.value)
  return items
})

async function showErrors() {
  await nextTick()
  summary.value?.scrollIntoView({ behavior: 'smooth', block: 'center' })
}

function clean(rec: Record<string, string>) {
  const out: Record<string, string> = {}
  for (const [k, v] of Object.entries(rec)) if (v.trim()) out[k] = v.trim()
  return out
}

async function save() {
  if (saving.value) return
  generalError.value = ''
  errors.value = validate()
  if (Object.keys(errors.value).length) return showErrors()

  const input: AppInput = {
    id: isEdit.value ? props.id! : form.id,
    name: form.name.trim(),
    description: form.description.trim(),
    owner: form.owner.trim(),
    contact: form.contact.trim(),
    category: form.category,
    runs_on: form.runs_on,
    version: form.version.trim(),
    start_here: form.start_here,
    access: form.access.trim(),
    health_url: form.health_url.trim(),
    urls: clean(form.urls),
    links: clean(form.links),
    visibility: form.visibility,
    groups: form.groups,
    icon_color: form.icon_color,
    icon: form.icon,
  }

  const details: DetailsInput = {
    description: input.description,
    contact: input.contact,
    version: input.version,
    access: input.access,
    health_url: input.health_url,
    urls: input.urls,
    links: input.links,
    icon_color: input.icon_color,
    icon: input.icon,
  }

  saving.value = true
  let app: App
  try {
    if (props.ownerMode) app = await api.updateDetails(props.id!, details)
    else app = isEdit.value ? await api.updateApp(props.id!, input) : await api.createApp(input)
  } catch (e) {
    saving.value = false
    if (e instanceof ApiError && Object.keys(e.fields).length) errors.value = { ...e.fields }
    else generalError.value = errorText(e, 'Could not save the app. Try again.')
    return showErrors()
  }

  try {
    if (pendingFile.value) app = await api.uploadIcon(app.id, pendingFile.value)
    else if (removeIcon.value && original.value?.has_icon) app = await api.deleteIcon(app.id)
  } catch (e) {
    saving.value = false
    const msg = `${app.name} was saved, but the icon change failed: ${errorText(e, 'try again')}`
    if (isEdit.value) {
      original.value = app
      generalError.value = msg
      return showErrors()
    }
    flash.value = { text: msg, error: true }
    return router.replace(`/admin/apps/${app.id}/edit`)
  }

  saving.value = false
  snapshot.value = ''
  flash.value = { text: isEdit.value ? `Changes to ${app.name} were saved.` : `${app.name} was added to the catalog.` }
  router.push(backTo.value)
}

function onDeleted() {
  flash.value = { text: `${original.value?.name ?? props.id} was deleted from the catalog.` }
  deleting.value = false
  router.push('/admin/apps')
}

// environment rows: the ones with a url, and production even when empty
const rows = ref<string[]>([])
function startRows() {
  const keys = envs.value.map((e) => e.key).filter((k) => form.urls[k]?.trim())
  const first = hasProd.value ? 'prod' : envs.value[0]?.key
  if (first && !keys.includes(first)) keys.unshift(first)
  return keys
}
const fixedRow = computed(() => (hasProd.value ? 'prod' : (envs.value[0]?.key ?? '')))
const unusedEnvs = computed(() => envs.value.filter((e) => !rows.value.includes(e.key)))
const envLabel = (k: string) => envs.value.find((e) => e.key === k)?.label ?? k
async function addEnv(key: string) {
  rows.value = envs.value.map((e) => e.key).filter((k) => k === key || rows.value.includes(k))
  await nextTick()
  document.getElementById(`f-url-${key}`)?.focus()
}
function removeEnv(key: string) {
  rows.value = rows.value.filter((k) => k !== key)
  const next = { ...form.urls }
  delete next[key]
  form.urls = next
  clearErr(`urls.${key}`)
}

// unsaved changes, per section for the save bar
const state = () =>
  JSON.stringify({
    basics: [form.name, form.id, form.description, form.category, form.runs_on, form.owner, form.contact, form.version.trim(), form.start_here],
    icon: [form.icon_color, form.icon],
    envs: clean(form.urls),
    health: form.health_url.trim(),
    links: clean(form.links),
    access: [form.visibility, [...form.groups].sort(), form.access.trim()],
  })
const snapshot = ref('')
const sectionNames: Record<string, string> = {
  basics: 'Basics',
  icon: 'Icon',
  envs: 'Environments',
  health: 'Health check',
  links: 'Links',
  access: 'Who can see it',
}
const changed = computed(() => {
  if (!snapshot.value) return []
  const now = JSON.parse(state())
  const was = JSON.parse(snapshot.value)
  const out = Object.keys(sectionNames).filter((k) => JSON.stringify(now[k]) !== JSON.stringify(was[k]))
  if ((pendingFile.value || removeIcon.value) && !out.includes('icon')) out.splice(1, 0, 'icon')
  return out.map((k) => sectionNames[k])
})
const dirty = computed(() => changed.value.length > 0)
const backTo = computed(() => (props.ownerMode ? `/apps/${encodeURIComponent(props.id ?? '')}` : '/admin/apps'))
function cancel() {
  router.push(backTo.value)
}

// test the health check before saving
const check = ref<Status | null>(null)
const checking = ref(false)
const checkError = ref('')
const canCheck = computed(() => !!form.health_url.trim() || !!Object.values(clean(form.urls)).length)
async function testHealth() {
  checking.value = true
  checkError.value = ''
  check.value = null
  try {
    check.value = await api.healthCheck({ health_url: form.health_url.trim(), urls: clean(form.urls) })
  } catch (e) {
    checkError.value = errorText(e, 'Could not run the check.')
  } finally {
    checking.value = false
  }
}
watch(
  () => [form.health_url, form.urls.prod],
  () => {
    check.value = null
    checkError.value = ''
  },
)

// the catalog card as it will look
const previewApp = computed<App>(() => ({
  ...(original.value ?? {
    has_icon: false,
    icon_version: 0,
    favorite: false,
    opens_30d: 0,
    can_edit: true,
    created_by: '',
    updated_by: '',
    created_at: '',
    updated_at: '',
    status: null,
  }),
  id: form.id || slugify(form.name) || 'app',
  name: form.name.trim() || 'App name',
  description: form.description.trim() || 'One line on what the app is for',
  owner: form.owner,
  contact: form.contact,
  category: form.category,
  runs_on: form.runs_on,
  version: form.version,
  start_here: form.start_here,
  access: form.access,
  health_url: form.health_url,
  urls: clean(form.urls),
  links: clean(form.links),
  visibility: form.visibility,
  groups: form.groups,
  icon_color: form.icon_color,
  icon: form.icon,
  has_icon: !!original.value?.has_icon && !removeIcon.value,
}))
const previewNote = computed(() => {
  if (form.visibility === 'everyone') return 'Every signed-in user will see this card.'
  if (!form.groups.length) return 'Only admins will see this card until you pick a group.'
  return `Members of ${form.groups.join(', ')} and admins will see this card.`
})
const optLabel = (list: { key: string; label: string }[], k: string) => list.find((o) => o.key === k)?.label ?? k

const crumb = computed(() => (isEdit.value ? original.value?.name || props.id : 'Add app'))
const sharedWith = computed(() => {
  if (form.visibility === 'everyone') return 'Everyone who signs in'
  if (!form.groups.length) return 'Only admins'
  return `Only members of ${form.groups.join(', ')}`
})
const heading = computed(() => (isEdit.value ? `Edit ${original.value?.name || 'app'}` : 'Add app'))
</script>

<template>
  <div :class="{ 'min-h-dvh': ownerMode }">
    <SiteHeader v-if="ownerMode" />
    <component
      :is="ownerMode ? 'main' : 'div'"
      class="flex flex-col gap-5"
      :class="{ 'mx-auto max-w-[1440px] px-4 pt-6 pb-24 sm:px-8 xl:px-10': ownerMode }"
    >
      <nav aria-label="Breadcrumb" class="flex items-center gap-1.5 text-sm text-subtle">
        <RouterLink :to="backTo" class="font-medium text-accent no-underline hover:text-accent-ink">
          {{ ownerMode ? crumb : 'Apps' }}
        </RouterLink>
        <Icon name="back" :size="14" class="rotate-180 text-faint" />
        <span>{{ ownerMode ? 'Edit' : isEdit ? crumb : 'New app' }}</span>
      </nav>
      <h1 class="m-0 text-[28px] font-semibold">{{ heading }}</h1>

      <div v-if="loading" aria-busy="true" aria-label="Loading the form" class="flex max-w-[800px] flex-col gap-4">
        <div v-for="i in 3" :key="i" class="flex h-40 flex-col gap-3 rounded-xl border border-line bg-white p-6">
          <span class="h-4 w-24 animate-pulse rounded bg-[#e9ebee]" />
          <span class="h-10 w-full animate-pulse rounded-lg bg-[#e9ebee]" />
          <span class="h-10 w-2/3 animate-pulse rounded-lg bg-[#e9ebee]" />
        </div>
      </div>
      <div v-else-if="notFound" class="card flex max-w-[640px] flex-col gap-3 p-6">
        <p class="m-0">There is no app with the ID <span class="font-mono">{{ id }}</span>.</p>
        <div>
          <RouterLink :to="ownerMode ? '/' : '/admin/apps'" class="btn btn-secondary">
            {{ ownerMode ? 'Back to catalog' : 'Back to apps' }}
          </RouterLink>
        </div>
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
        <div v-if="summaryItems.length" ref="summary" role="alert" class="rounded-[10px] bg-warn-soft px-4 py-3.5 text-sm text-warn">
          <p class="m-0 mb-1.5 font-semibold">Fix these before {{ isEdit ? 'saving' : 'adding the app' }}:</p>
          <ul class="m-0 flex list-disc flex-col gap-1 pl-5">
            <li v-for="(err, i) in summaryItems" :key="i">{{ err }}</li>
          </ul>
        </div>

        <div class="flex flex-col gap-6 lg:flex-row lg:items-start">
          <form id="app-form" novalidate class="flex min-w-0 flex-1 flex-col gap-4" @submit.prevent="save">
            <section aria-labelledby="sec-basics" class="card flex flex-col gap-4 p-6">
              <h2 id="sec-basics" class="section-title text-[17px]">Basics</h2>
              <p v-if="ownerMode" class="hint -mt-2 mb-0">
                Only admins can change the name, category, runs on, owner team and Start here.
              </p>
              <div class="grid gap-4 sm:grid-cols-2">
                <div class="flex flex-col gap-1.5">
                  <label for="f-name" class="label">Name</label>
                  <input
                    id="f-name"
                    v-model="form.name"
                    type="text"
                    placeholder="Grafana"
                    class="field"
                    :readonly="ownerMode"
                    :aria-invalid="!!errors.name"
                    @input="onName"
                  />
                  <span v-if="errors.name" class="field-error">{{ errors.name }}</span>
                </div>
                <div class="flex flex-col gap-1.5">
                  <label for="f-id" class="label">App ID</label>
                  <input
                    id="f-id"
                    v-model="form.id"
                    type="text"
                    placeholder="grafana"
                    spellcheck="false"
                    autocomplete="off"
                    class="field font-mono text-[13px]"
                    :readonly="isEdit"
                    :aria-invalid="!!errors.id"
                    @input="onId"
                  />
                  <span v-if="errors.id" class="field-error">{{ errors.id }}</span>
                  <span v-else class="hint">
                    {{ isEdit ? "Can't be changed after the app is created." : 'Filled in from the name. Lowercase letters, numbers and dashes.' }}
                  </span>
                </div>
              </div>
              <div class="flex flex-col gap-1.5">
                <label for="f-desc" class="label">Description</label>
                <textarea
                  id="f-desc"
                  v-model="form.description"
                  rows="2"
                  placeholder="One line on what the app is for"
                  class="field resize-y"
                />
              </div>
              <div class="grid gap-4 sm:grid-cols-2">
                <div class="flex flex-col gap-1.5">
                  <label for="f-cat" class="label">Category</label>
                  <Select
                    id="f-cat"
                    v-model="form.category"
                    :options="categoryOptions"
                    :disabled="ownerMode"
                    :invalid="!!errors.category"
                    @change="clearErr('category')"
                  />
                  <span v-if="errors.category" class="field-error">{{ errors.category }}</span>
                </div>
                <div class="flex flex-col gap-1.5">
                  <label for="f-runs" class="label">Runs on</label>
                  <Select
                    id="f-runs"
                    v-model="form.runs_on"
                    :options="runsOnOptions"
                    :disabled="ownerMode"
                    :invalid="!!errors.runs_on"
                    @change="clearErr('runs_on')"
                  />
                  <span v-if="errors.runs_on" class="field-error">{{ errors.runs_on }}</span>
                </div>
                <div class="flex flex-col gap-1.5">
                  <label for="f-owner" class="label">Owner team</label>
                  <Select
                    id="f-owner"
                    v-model="form.owner"
                    :options="ownerOptions"
                    placeholder="Pick a team"
                    :disabled="ownerMode"
                    :invalid="!!errors.owner"
                    @change="clearErr('owner')"
                  />
                  <span v-if="errors.owner" class="field-error">{{ errors.owner }}</span>
                </div>
                <div class="flex flex-col gap-1.5">
                  <label for="f-contact" class="label">Contact <span class="font-normal text-subtle">(optional)</span></label>
                  <input
                    id="f-contact"
                    v-model="form.contact"
                    type="text"
                    placeholder="team@yourcompany.id"
                    class="field"
                    :aria-invalid="!!errors.contact"
                    @input="clearErr('contact')"
                  />
                  <span v-if="errors.contact" class="field-error">{{ errors.contact }}</span>
                </div>
                <div class="flex flex-col gap-1.5">
                  <label for="f-version" class="label">Version <span class="font-normal text-subtle">(optional)</span></label>
                  <input
                    id="f-version"
                    v-model="form.version"
                    type="text"
                    placeholder="v1.4.2"
                    maxlength="40"
                    spellcheck="false"
                    autocomplete="off"
                    class="field font-mono text-[13px]"
                    :aria-invalid="!!errors.version"
                    @input="clearErr('version')"
                  />
                  <span v-if="errors.version" class="field-error">{{ errors.version }}</span>
                </div>
                <label
                  class="flex items-start gap-2.5 self-end pb-1 sm:min-h-11 sm:items-center sm:pb-0"
                  :class="ownerMode ? 'cursor-not-allowed' : 'cursor-pointer'"
                >
                  <input v-model="form.start_here" type="checkbox" class="check mt-0.5 sm:mt-0" :disabled="ownerMode" />
                  <span class="flex flex-col">
                    <span class="text-sm font-medium">Start here</span>
                    <span class="hint">List it under Start here for new people.</span>
                  </span>
                </label>
              </div>
            </section>

            <section aria-labelledby="sec-icon" class="card flex flex-col gap-3.5 p-6">
              <h2 id="sec-icon" class="section-title text-[17px]">Icon</h2>
              <div class="flex flex-wrap items-center gap-x-5 gap-y-3">
                <AppIcon :app="iconPreviewApp" :size="64" :src="previewURL || undefined" :no-upload="!showUploaded" />
                <fieldset v-if="!showUploaded" class="m-0 flex flex-wrap items-center gap-2 border-0 p-0">
                  <legend class="sr-only">Default icon color</legend>
                  <button
                    v-for="sw in swatches"
                    :key="sw.key"
                    type="button"
                    :aria-pressed="form.icon_color === sw.key"
                    :aria-label="sw.label"
                    :title="sw.label"
                    class="flex h-7 cursor-pointer items-center justify-center rounded-full text-[11px] font-semibold"
                    :class="sw.text ? 'px-2.5' : 'w-7'"
                    :style="{
                      background: sw.bg,
                      color: sw.fg,
                      boxShadow: form.icon_color === sw.key ? '0 0 0 2px #FFFFFF, 0 0 0 4px #15171C' : 'none',
                    }"
                    @click="form.icon_color = sw.key"
                  >
                    <Icon v-if="!sw.text && form.icon_color === sw.key" name="check" :size="14" />
                    {{ sw.text }}
                  </button>
                </fieldset>
                <span v-else class="text-[13px] text-subtle">
                  {{ pendingFile ? pendingFile.name : 'Uploaded icon' }}
                </span>
                <div class="ml-auto flex flex-wrap gap-2">
                  <button v-if="showUploaded || form.icon" type="button" class="btn btn-secondary" @click="useDefaultIcon">
                    Use initials
                  </button>
                  <button type="button" class="btn btn-secondary" @click="picking = true">
                    <Icon name="grid" :size="16" />
                    Choose icon
                  </button>
                  <label class="btn btn-secondary relative">
                    <Icon name="upload" :size="16" />
                    Upload
                    <input
                      ref="fileInput"
                      type="file"
                      accept="image/png,image/svg+xml,.png,.svg"
                      class="absolute size-px overflow-hidden opacity-0"
                      @change="onFile"
                    />
                  </label>
                </div>
              </div>
              <p v-if="iconError" role="alert" class="field-error m-0 text-[13px]">{{ iconError }}</p>
              <p v-else-if="removeIcon && !form.icon" class="m-0 text-[13px] text-subtle">The uploaded icon will be removed when you save.</p>
              <p v-else-if="form.icon && !showUploaded" class="hint m-0">
                Tabler icon <span class="font-mono">{{ form.icon }}</span> on the color you pick.
                <template v-if="removeIcon"> The uploaded icon will be removed when you save.</template>
              </p>
              <p v-else class="hint m-0">
                Pick a color for the initials, choose an icon, or upload a square PNG or SVG (128 px or more, up to 256 KB).
                <template v-if="settings?.icon_color_mode === 'accent'"> Settings use the accent color for every app right now.</template>
              </p>
            </section>

            <section aria-labelledby="sec-envs" class="card flex flex-col gap-3 p-6">
              <div class="flex flex-wrap items-end justify-between gap-2">
                <div class="flex flex-col gap-0.5">
                  <h2 id="sec-envs" class="section-title text-[17px]">Environments</h2>
                  <p class="m-0 text-[13px] text-subtle">
                    One URL per environment.<template v-if="fixedRow"> Open app goes to {{ envLabel(fixedRow) }}.</template>
                  </p>
                </div>
                <span v-if="envs.length" class="text-[12.5px] text-faint">{{ rows.length }} of {{ envs.length }} used</span>
              </div>
              <p v-if="!envs.length" class="m-0 text-sm text-subtle">
                No environments yet.
                <template v-if="!ownerMode">
                  Add them in <RouterLink to="/admin/settings?tab=catalog" class="link">Settings</RouterLink>.
                </template>
              </p>
              <div v-for="key in rows" :key="key" class="flex items-start gap-2">
                <label
                  :for="`f-url-${key}`"
                  class="flex h-10 w-[168px] shrink-0 items-center gap-1.5 rounded-lg border border-line bg-panel px-3 text-[13px] font-medium"
                >
                  <span class="min-w-0 flex-1 truncate">{{ envLabel(key) }}</span>
                  <span v-if="key === fixedRow" class="chip-blue px-1.5 py-px text-[11px]">Open app</span>
                </label>
                <div class="flex min-w-0 flex-1 flex-col gap-1.5">
                  <input
                    :id="`f-url-${key}`"
                    v-model="form.urls[key]"
                    type="text"
                    inputmode="url"
                    spellcheck="false"
                    :placeholder="urlPlaceholder(key)"
                    class="field h-10 font-mono text-[13px]"
                    :aria-invalid="!!errors[`urls.${key}`]"
                    @input="clearErr(`urls.${key}`)"
                  />
                  <span v-if="errors[`urls.${key}`]" class="field-error">{{ errors[`urls.${key}`] }}</span>
                </div>
                <button
                  v-if="key !== fixedRow"
                  type="button"
                  class="icon-btn size-10"
                  :aria-label="`Remove ${envLabel(key)} URL`"
                  @click="removeEnv(key)"
                >
                  <Icon name="x" :size="16" />
                </button>
                <span v-else class="size-10 shrink-0" />
              </div>
              <div v-if="envs.length" class="flex flex-wrap items-center justify-between gap-3">
                <Popover
                  v-if="unusedEnvs.length"
                  label="Add environment"
                  panel-class="w-56"
                  button-class="inline-flex h-9 cursor-pointer items-center gap-1.5 rounded-lg border border-dashed border-field bg-white px-3 text-[13px] font-medium text-accent hover:border-accent"
                >
                  <template #trigger>
                    <Icon name="plus" :size="16" />
                    Add environment
                  </template>
                  <template #default="{ close }">
                    <span class="block px-2.5 pt-1 pb-1.5 text-[11.5px] font-semibold text-faint">Not used yet</span>
                    <button
                      v-for="e in unusedEnvs"
                      :key="e.key"
                      type="button"
                      role="menuitem"
                      class="menu-item"
                      @click="(close(false), addEnv(e.key))"
                    >
                      {{ e.label }}
                    </button>
                  </template>
                </Popover>
                <span v-else class="text-[13px] text-subtle">Every environment has a URL.</span>
                <span v-if="!ownerMode" class="text-[12.5px] text-subtle">
                  Environments are set in <RouterLink to="/admin/settings?tab=catalog" class="font-medium text-accent no-underline hover:underline">Settings</RouterLink>
                </span>
              </div>
            </section>

            <section aria-labelledby="sec-health" class="card flex flex-col gap-3 p-6">
              <div class="flex flex-col gap-0.5">
                <h2 id="sec-health" class="section-title text-[17px]">Health check</h2>
                <p class="m-0 text-[13px] text-subtle">Optional. The portal checks this URL every minute and shows the result in the catalog.</p>
              </div>
              <div class="flex gap-2.5">
                <label for="f-health" class="sr-only">Health check URL</label>
                <input
                  id="f-health"
                  v-model="form.health_url"
                  type="text"
                  spellcheck="false"
                  placeholder="/healthz or a full URL"
                  class="field h-10 font-mono text-[13px]"
                  :aria-invalid="!!errors.health_url"
                  @input="clearErr('health_url')"
                />
                <button
                  v-if="!ownerMode"
                  type="button"
                  class="btn btn-secondary btn-sm h-10"
                  :disabled="!canCheck || checking"
                  @click="testHealth"
                >
                  {{ checking ? 'Checking...' : 'Test now' }}
                </button>
              </div>
              <span v-if="errors.health_url" class="field-error">{{ errors.health_url }}</span>
              <p
                v-if="check"
                role="status"
                class="m-0 flex flex-wrap items-center gap-2 rounded-lg px-3 py-2 text-[13px]"
                :class="check.up ? 'bg-ok-soft text-ok' : 'bg-warn-soft text-warn'"
                :title="statusTitle(check)"
              >
                <span class="size-2 rounded-full" :class="check.up ? 'bg-ok' : 'bg-down'" />
                <span class="font-semibold">{{ check.up ? 'Up' : 'Down' }}</span>
                <span>
                  {{ check.up ? `HTTP ${check.http_status}, ${check.latency_ms} ms` : check.error || `HTTP ${check.http_status}` }},
                  checked <span class="font-mono text-xs break-all">{{ check.target }}</span>
                </span>
              </p>
              <p v-else-if="checkError" role="alert" class="status-error">{{ checkError }}</p>
              <span class="hint">A full URL, or a path like /healthz on the {{ fixedRow ? envLabel(fixedRow) : 'main' }} URL.</span>
            </section>

            <section aria-labelledby="sec-links" class="card flex flex-col gap-3 p-6">
              <div class="flex flex-col gap-0.5">
                <h2 id="sec-links" class="section-title text-[17px]">Links</h2>
                <p class="m-0 text-[13px] text-subtle">Optional. Shown on the app page.</p>
              </div>
              <div v-for="l in LINK_KINDS" :key="l.key" class="flex flex-wrap items-start gap-x-4 gap-y-1.5">
                <label :for="`f-link-${l.key}`" class="label flex h-10 w-[130px] items-center">{{ l.label }}</label>
                <div class="flex min-w-0 flex-[1_1_240px] flex-col gap-1.5">
                  <input
                    :id="`f-link-${l.key}`"
                    v-model="form.links[l.key]"
                    type="text"
                    inputmode="url"
                    spellcheck="false"
                    :placeholder="linkPlaceholder(l.key)"
                    class="field h-10 font-mono text-[13px]"
                    :aria-invalid="!!errors[`links.${l.key}`]"
                    @input="clearErr(`links.${l.key}`)"
                  />
                  <span v-if="errors[`links.${l.key}`]" class="field-error">{{ errors[`links.${l.key}`] }}</span>
                </div>
              </div>
            </section>

            <section aria-labelledby="sec-access" class="card flex flex-col gap-3.5 p-6">
              <h2 id="sec-access" class="section-title text-[17px]">Who can see this app</h2>
              <div v-if="ownerMode" class="flex flex-col gap-1">
                <span class="text-sm">{{ sharedWith }}</span>
                <span class="hint">Only admins can change who sees this app.</span>
              </div>
              <template v-else>
                <div class="flex flex-col">
                  <label class="flex min-h-11 cursor-pointer items-center gap-2.5 text-sm">
                    <input v-model="form.visibility" type="radio" name="vis" value="everyone" class="check" @change="clearErr('visibility')" />
                    Everyone who signs in
                  </label>
                  <label class="flex min-h-11 cursor-pointer items-center gap-2.5 text-sm">
                    <input v-model="form.visibility" type="radio" name="vis" value="groups" class="check" @change="clearErr('visibility')" />
                    Only members of these groups
                  </label>
                </div>
                <span v-if="errors.visibility" class="field-error">{{ errors.visibility }}</span>
                <fieldset v-if="form.visibility === 'groups'" class="m-0 rounded-[10px] border border-line px-4 py-1">
                  <legend class="sr-only">Groups</legend>
                  <p v-if="!groups.length" class="m-0 py-3 text-sm text-subtle">No groups yet.</p>
                  <label
                    v-for="g in groups"
                    :key="g.name"
                    class="flex min-h-12 cursor-pointer items-center gap-3 border-b border-line-soft py-1.5 last:border-b-0"
                  >
                    <input type="checkbox" class="check" :checked="form.groups.includes(g.name)" @change="toggleGroup(g.name)" />
                    <span class="flex min-w-0 flex-1 flex-col gap-px">
                      <span class="font-mono text-[13.5px]">{{ g.name }}</span>
                      <span v-if="g.description" class="text-[12.5px] text-subtle">{{ g.description }}</span>
                    </span>
                    <span class="text-[13px] whitespace-nowrap text-subtle">{{ plural(g.members.length, 'member') }}</span>
                  </label>
                </fieldset>
                <span v-if="errors.groups" class="field-error">{{ errors.groups }}</span>
                <p class="m-0 flex items-start gap-1.5 text-[13px]" :class="noGroupsWarning ? 'font-medium text-warn' : 'text-subtle'">
                  <Icon v-if="noGroupsWarning" name="info" :size="15" class="mt-px" />
                  {{ noGroupsWarning ? `No group picked yet, so only admins will see ${form.name.trim() || 'this app'}.` : visSummary }}
                </p>
              </template>
              <div class="flex flex-col gap-1.5 border-t border-line-soft pt-3.5">
                <label for="f-access" class="label">Access note <span class="font-normal text-subtle">(optional)</span></label>
                <textarea
                  id="f-access"
                  v-model="form.access"
                  rows="2"
                  placeholder="How to get a login inside the app"
                  class="field resize-y"
                />
                <span class="hint">Shown on the app page as "Getting access".</span>
              </div>
            </section>

            <section
              v-if="isEdit && original && !ownerMode"
              aria-labelledby="sec-danger"
              class="flex flex-wrap items-center gap-x-6 gap-y-3 rounded-xl border border-warn-line bg-white p-6"
            >
              <div class="flex flex-[1_1_320px] flex-col gap-1">
                <h2 id="sec-danger" class="m-0 text-base font-semibold text-warn">Delete this app</h2>
                <p class="m-0 text-sm text-muted">Removes it from the catalog and from every group. The app itself keeps running.</p>
              </div>
              <button type="button" class="btn btn-danger-outline px-[18px] font-semibold" @click="deleting = true">Delete app</button>
            </section>
          </form>

          <aside class="flex flex-col gap-2.5 lg:sticky lg:top-6 lg:w-[320px] lg:shrink-0">
            <h2 class="eyebrow m-0">Preview in the catalog</h2>
            <div inert class="select-none">
              <AppCard
                :app="previewApp"
                :envs="envs"
                :category="optLabel(categories, form.category)"
                :runs-on="optLabel(runsOn, form.runs_on)"
                :admin="!ownerMode"
              />
            </div>
            <p class="m-0 flex items-start gap-2 px-0.5 text-[12.5px] leading-normal text-subtle">
              <Icon name="info" :size="15" class="mt-px shrink-0" />
              Updates as you type. {{ previewNote }}
            </p>
          </aside>
        </div>
      </template>

      <SaveBar
        :show="!loading && !notFound && !loadError && (dirty || !isEdit)"
        :label="isEdit ? 'Unsaved changes' : 'New app'"
        :detail="isEdit ? changed.join(', ') : 'Not added yet'"
        :saving="saving"
        :action="isEdit ? 'Save changes' : 'Add app'"
        form="app-form"
        @cancel="cancel"
      />
      <IconPicker
        v-if="picking"
        :query="form.name"
        :current="form.icon"
        :on-file="acceptFile"
        @tabler="useTabler"
        @close="picking = false"
      />
      <DeleteAppModal v-if="deleting && original" :app="original" :groups="groups" @close="deleting = false" @deleted="onDeleted" />
    </component>
  </div>
</template>
