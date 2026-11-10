<script setup lang="ts">
import { computed, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import AppIcon from '@/components/AppIcon.vue'
import Icon from '@/components/Icon.vue'
import { ApiError, type ImportPlan, api, errorText, plural } from '@/api'
import { flash, settings } from '@/session'

const router = useRouter()
const maxBytes = 1 << 20

const fileName = ref('')
const fileSize = ref(0)
const text = ref('')
const plan = ref<ImportPlan | null>(null)
const checking = ref(false)
const importing = ref(false)
const error = ref('')
const existing = ref<'skip' | 'update'>('skip')
const dragging = ref(false)
const input = ref<HTMLInputElement | null>(null)

async function pick(file: File | undefined) {
  if (!file) return
  error.value = ''
  plan.value = null
  if (file.size > maxBytes) {
    error.value = `${file.name} is over 1 MB. Split it into smaller files.`
    return
  }
  fileName.value = file.name
  fileSize.value = file.size
  text.value = await file.text()
  await check()
}

async function check() {
  checking.value = true
  error.value = ''
  try {
    plan.value = await api.previewImport(text.value)
    existing.value = 'skip'
  } catch (e) {
    error.value = errorText(e, 'Could not check the file.')
  } finally {
    checking.value = false
  }
}

function onDrop(e: DragEvent) {
  dragging.value = false
  pick(e.dataTransfer?.files[0])
}

function reset() {
  plan.value = null
  text.value = ''
  fileName.value = ''
  error.value = ''
  if (input.value) input.value.value = ''
}

const apps = computed(() => plan.value?.apps ?? [])
const problems = computed(() => plan.value?.problems ?? [])
const newCount = computed(() => apps.value.filter((a) => !a.exists).length)
const existCount = computed(() => apps.value.filter((a) => a.exists).length)
const toSave = computed(() => newCount.value + (existing.value === 'update' ? existCount.value : 0))
const create = computed(() => {
  const c = plan.value?.create
  if (!c) return []
  return [
    { label: 'Categories', items: c.categories.map((o) => o.label) },
    { label: 'Runs on', items: c.runs_on.map((o) => o.label) },
    { label: 'Environments', items: c.environments.map((o) => o.label) },
    { label: 'Groups', items: c.groups, mono: true },
  ].filter((r) => r.items.length)
})
const problemsFor = (id: string) => problems.value.filter((p) => p.app === id)

function statusOf(a: ImportPlan['apps'][number]) {
  if (a.problems) return { text: plural(a.problems, 'problem'), cls: 'chip-warn' }
  if (!a.exists) return { text: 'New', cls: 'chip-ok' }
  return existing.value === 'update' ? { text: 'Will update', cls: 'chip-blue' } : { text: 'Will skip', cls: 'chip' }
}

const sizeText = computed(() => (fileSize.value < 1024 ? `${fileSize.value} bytes` : `${Math.ceil(fileSize.value / 1024)} KB`))

async function run() {
  if (!toSave.value || problems.value.length || importing.value) return
  importing.value = true
  error.value = ''
  try {
    const res = await api.runImport(text.value, existing.value)
    const parts = [`added ${res.added.length}`]
    if (res.updated.length) parts.push(`updated ${res.updated.length}`)
    if (res.skipped.length) parts.push(`skipped ${res.skipped.length} that already exist`)
    flash.value = { text: `Imported ${fileName.value}: ${parts.join(', ')}.` }
    // the import can add options, the apps list shows their labels
    settings.value = await api.settings().catch(() => settings.value)
    router.push('/admin/apps')
  } catch (e) {
    const p = e instanceof ApiError ? (e.body as { plan?: ImportPlan } | null)?.plan : undefined
    if (p) plan.value = p
    error.value = errorText(e, 'Could not import the file.')
  } finally {
    importing.value = false
  }
}
</script>

<template>
  <div class="flex flex-col gap-5">
    <nav aria-label="Breadcrumb" class="flex items-center gap-1.5 text-sm text-subtle">
      <RouterLink to="/admin/apps" class="font-medium text-accent no-underline hover:text-accent-ink">Apps</RouterLink>
      <Icon name="back" :size="14" class="rotate-180 text-faint" />
      <span>Import</span>
    </nav>
    <div class="flex flex-col gap-1">
      <h1 class="m-0 text-[28px] font-semibold">Import apps</h1>
      <p class="m-0 text-sm text-subtle">Add many apps at once from a YAML file. Nothing is saved until you confirm.</p>
    </div>

    <template v-if="!plan">
      <label
        class="card flex cursor-pointer flex-col items-center gap-3 border-2 border-dashed px-6 py-12 text-center"
        :class="dragging ? 'border-accent bg-accent-soft' : 'border-line hover:border-subtle'"
        @dragover.prevent="dragging = true"
        @dragleave="dragging = false"
        @drop.prevent="onDrop"
      >
        <span class="flex size-12 items-center justify-center rounded-full bg-accent-soft text-accent">
          <Icon name="upload" :size="22" />
        </span>
        <span v-if="checking" class="text-[15px] font-semibold" role="status">Checking {{ fileName }}...</span>
        <template v-else>
          <span class="text-[15px] font-semibold">Drop a YAML file here, or <span class="text-accent underline">choose one</span></span>
          <span class="text-[13px] text-subtle">One file with a list of apps, up to 1 MB</span>
        </template>
        <input
          ref="input"
          type="file"
          accept=".yaml,.yml,application/yaml,text/yaml"
          class="sr-only"
          :disabled="checking"
          @change="pick(($event.target as HTMLInputElement).files?.[0])"
        />
      </label>

      <p v-if="error" role="alert" class="status-error">{{ error }}</p>

      <section aria-labelledby="fmt-h" class="card flex flex-col gap-3 px-6 py-5">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <h2 id="fmt-h" class="section-title text-base">File format</h2>
          <a href="/import-example.yaml" download class="btn btn-secondary btn-sm">Download example</a>
        </div>
        <pre class="m-0 overflow-x-auto rounded-lg bg-canvas px-4 py-3 font-mono text-[12.5px] leading-relaxed text-muted">version: 1
apps:
  - id: grafana              # required, lowercase and dashes
    name: Grafana            # required
    owner: platform          # required, a group name
    category: Monitoring     # key or label, new ones are created
    runs_on: kubernetes
    version: v10.4.2
    start_here: true         # list it under Start here
    visible_to: everyone     # or a list of groups, [] for admins only
    icon_color: orange       # auto, blue, teal, purple, orange, green, gray
    icon: chart-line         # a name from tabler.io/icons
    urls:
      prod: https://grafana.internal.example
    links:
      docs: https://grafana.com/docs   # repo, docs or ci</pre>
        <p class="m-0 text-[13px] text-subtle">
          Also: <span class="font-mono">description</span>, <span class="font-mono">contact</span>,
          <span class="font-mono">access</span> and <span class="font-mono">health_url</span>. Uploaded icons can't be set from a file.
        </p>
      </section>
    </template>

    <template v-else>
      <div class="card flex flex-wrap items-center gap-3 px-5 py-3.5">
        <span class="flex size-9 items-center justify-center rounded-lg bg-chip text-muted"><Icon name="docs" :size="18" /></span>
        <span class="flex min-w-0 flex-1 flex-col">
          <span class="truncate text-sm font-semibold">{{ fileName }}</span>
          <span class="text-[13px] text-subtle">
            {{ sizeText }}, {{ plural(apps.length, 'app') }}<template v-if="apps.length">: {{ newCount }} new, {{ existCount }} already in the portal</template>
          </span>
        </span>
        <button type="button" class="btn btn-secondary btn-sm" @click="reset">Choose another file</button>
      </div>

      <section v-if="problems.length" aria-labelledby="prob-h" class="card overflow-hidden border-warn-line">
        <div class="flex items-start gap-3 bg-warn-soft px-5 py-4 text-warn">
          <Icon name="info" :size="20" class="mt-0.5 shrink-0" />
          <div class="flex flex-col gap-0.5">
            <h2 id="prob-h" class="m-0 text-[15px] font-semibold">Fix {{ plural(problems.length, 'problem') }} in the file</h2>
            <p class="m-0 text-[13px] text-warn-ink">Nothing is imported until the whole file is clean. Fix it, then choose it again.</p>
          </div>
        </div>
        <ul class="m-0 max-h-[360px] list-none overflow-y-auto p-0">
          <li
            v-for="(p, i) in problems"
            :key="i"
            class="grid grid-cols-[72px_minmax(0,140px)_1fr] items-baseline gap-3 border-t border-line-soft px-5 py-2.5 text-sm"
          >
            <span class="font-mono text-xs text-subtle">{{ p.line ? `Line ${p.line}` : 'File' }}</span>
            <span class="truncate font-mono text-[13px]">{{ p.app }}</span>
            <span>{{ p.message }}</span>
          </li>
        </ul>
      </section>

      <section v-if="create.length && !problems.length" aria-labelledby="new-h" class="card flex flex-col gap-3 px-6 py-5">
        <div class="flex flex-col gap-0.5">
          <h2 id="new-h" class="section-title text-base">Also adds</h2>
          <p class="m-0 text-[13px] text-subtle">The file uses these, and the portal doesn't have them yet.</p>
        </div>
        <dl class="m-0 grid grid-cols-[120px_1fr] gap-x-4 gap-y-2.5">
          <template v-for="r in create" :key="r.label">
            <dt class="text-sm text-subtle">{{ r.label }}</dt>
            <dd class="m-0 flex flex-wrap gap-1.5">
              <span v-for="it in r.items" :key="it" :class="r.mono ? 'chip-group' : 'chip-blue'">{{ it }}</span>
            </dd>
          </template>
        </dl>
      </section>

      <fieldset v-if="existCount && !problems.length" class="card m-0 flex flex-col gap-2.5 px-6 py-5">
        <legend class="sr-only">Apps that already exist</legend>
        <p class="m-0 text-base font-semibold" aria-hidden="true">
          {{ existCount === 1 ? '1 app is already in the portal' : `${existCount} apps are already in the portal` }}
        </p>
        <label
          v-for="o in [
            { v: 'skip', title: 'Skip them', help: 'Keep them as they are. Only new apps are added.' },
            { v: 'update', title: 'Update them from the file', help: 'Every field takes the value in the file, left out fields are cleared. Uploaded icons stay.' },
          ]"
          :key="o.v"
          class="flex cursor-pointer items-start gap-3 rounded-lg border px-4 py-3"
          :class="existing === o.v ? 'border-accent bg-accent-soft' : 'border-line hover:bg-hover'"
        >
          <input v-model="existing" type="radio" name="existing" :value="o.v" class="mt-1 accent-accent" />
          <span class="flex flex-col gap-0.5">
            <span class="text-sm font-semibold">{{ o.title }}</span>
            <span class="text-[13px] text-subtle">{{ o.help }}</span>
          </span>
        </label>
      </fieldset>

      <section v-if="apps.length" aria-labelledby="apps-h" class="card overflow-x-auto">
        <h2 id="apps-h" class="sr-only">Apps in the file</h2>
        <ul class="m-0 min-w-[640px] list-none p-0">
          <li
            v-for="a in apps"
            :key="`${a.line}-${a.id}`"
            class="grid grid-cols-[minmax(220px,1.6fr)_minmax(160px,1.2fr)_minmax(120px,auto)] items-center gap-4 border-b border-line-soft px-5 py-3 last:border-b-0"
          >
            <span class="flex min-w-0 items-center gap-3">
              <AppIcon :app="{ id: a.id || '?', name: a.name || a.id || '?', icon_color: a.icon_color, icon: a.icon }" :size="32" no-upload />
              <span class="flex min-w-0 flex-col gap-px">
                <span class="truncate text-sm font-semibold" :class="{ 'text-faint': !a.name }">{{ a.name || 'No name' }}</span>
                <span class="truncate font-mono text-xs text-subtle">{{ a.id || 'no id' }}, line {{ a.line }}</span>
              </span>
            </span>
            <span class="flex flex-wrap gap-1">
              <span v-if="a.visibility === 'everyone'" class="chip-ok">Everyone</span>
              <template v-else>
                <span v-for="g in a.groups" :key="g" class="chip-group">{{ g }}</span>
                <span v-if="!a.groups.length" class="chip-warn">Admins only</span>
              </template>
            </span>
            <span class="justify-self-end">
              <span :class="statusOf(a).cls" :title="problemsFor(a.id).map((p) => p.message).join('\n')">{{ statusOf(a).text }}</span>
            </span>
            <p v-if="a.owner_note" class="col-span-3 m-0 -mt-2 flex items-center gap-1.5 text-[13px] text-warn">
              <Icon name="info" :size="14" class="shrink-0" />
              {{ a.owner_note }}
            </p>
          </li>
        </ul>
      </section>

      <p v-if="error" role="alert" class="status-error">{{ error }}</p>

      <div class="flex flex-wrap items-center gap-3">
        <button type="button" class="btn btn-primary px-[18px]" :disabled="!toSave || problems.length > 0 || importing" @click="run">
          {{ importing ? 'Importing...' : problems.length ? 'Import' : toSave ? `Import ${plural(toSave, 'app')}` : 'Nothing to import' }}
        </button>
        <RouterLink to="/admin/apps" class="btn btn-ghost">Cancel</RouterLink>
        <span v-if="!problems.length && !toSave && existCount" class="text-[13px] text-subtle">
          Every app is already in the portal. Choose Update to replace them.
        </span>
      </div>
    </template>
  </div>
</template>
