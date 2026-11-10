<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import AppIcon from '@/components/AppIcon.vue'
import Icon from '@/components/Icon.vue'
import SiteHeader from '@/components/SiteHeader.vue'
import NoAccess from '@/views/NoAccess.vue'
import NotFoundView from '@/views/NotFoundView.vue'
import {
  type App,
  ApiError,
  LINK_KINDS,
  type NoAccessBody,
  api,
  copyText,
  displayURL,
  errorText,
  fmtDate,
  hostOf,
  href,
  isAbort,
  isEmail,
  orderedURLs,
  primaryURL,
  timeAgo,
  trackOpen,
} from '@/api'
import { flash, isAdmin, optionLabel, setTitle, settings } from '@/session'

const props = defineProps<{ id: string }>()

const app = ref<App>()
const notFound = ref(false)
const noAccess = ref<NoAccessBody>()
const error = ref('')
const loading = ref(true)
// "Changes were saved" after an edit
const toast = ref(flash.value && !flash.value.error ? flash.value.text : '')
flash.value = null

// other apps in the same category, the page works fine without them
const related = ref<App[]>([])
async function loadRelated(a: App, signal: AbortSignal) {
  if (!a.category) return
  try {
    const res = await api.apps({ category: a.category }, signal)
    related.value = res.apps.filter((x) => x.id !== a.id).slice(0, 3)
  } catch {
    related.value = []
  }
}

let ctrl: AbortController | undefined
async function load() {
  ctrl?.abort()
  const c = (ctrl = new AbortController())
  app.value = undefined
  related.value = []
  notFound.value = false
  noAccess.value = undefined
  error.value = ''
  loading.value = true
  try {
    app.value = await api.app(props.id, c.signal)
    setTitle(app.value.name)
    loadRelated(app.value, c.signal)
  } catch (e) {
    if (isAbort(e)) return
    if (e instanceof ApiError && e.status === 404) {
      notFound.value = true
      setTitle('App not found')
    } else if (e instanceof ApiError && e.status === 403) {
      const body = e.body as NoAccessBody | null
      noAccess.value = {
        error: body?.error ?? '',
        app: body?.app ?? { id: props.id, name: props.id },
        groups: body?.groups ?? [],
        admin_email: body?.admin_email ?? '',
        request: body?.request ?? null,
      }
      setTitle(noAccess.value.app.name)
    } else error.value = errorText(e, 'Check your connection and try again. If it keeps failing, the portal server may be down.')
  } finally {
    if (ctrl === c) loading.value = false
  }
}
watch(() => props.id, load, { immediate: true })
watch(
  () => props.id,
  () => (toast.value = ''),
)
onBeforeUnmount(() => ctrl?.abort())

const envs = computed(() => settings.value?.environments ?? [])
const urls = computed(() => (app.value ? orderedURLs(app.value, envs.value) : []))
const primary = computed(() => (app.value ? (primaryURL(app.value, envs.value) ?? '') : ''))
const openURL = computed(() => (primary.value ? href(primary.value) : ''))
const links = computed(() =>
  app.value ? LINK_KINDS.filter((k) => app.value!.links[k.key]).map((k) => ({ ...k, url: app.value!.links[k.key]! })) : [],
)
const linkIcon = { repo: 'repo', docs: 'docs', ci: 'ci' } as const
const envLabel = (k: string) => optionLabel(envs.value, k)

const editTo = computed(() => {
  const id = encodeURIComponent(app.value?.id ?? '')
  return isAdmin.value ? `/admin/apps/${id}/edit` : `/apps/${id}/edit`
})

const status = computed(() => app.value?.status ?? null)
const showHealth = computed(() => !!status.value || !!app.value?.health_url)
const when = (iso: string) =>
  new Date(iso).toLocaleString('en-GB', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })

const favBusy = ref(false)
async function toggleFav() {
  const a = app.value
  if (!a || favBusy.value) return
  favBusy.value = true
  a.favorite = !a.favorite
  try {
    await api.favorite(a.id, a.favorite)
  } catch (e) {
    a.favorite = !a.favorite
    error.value = errorText(e, 'Could not update favorites.')
  } finally {
    favBusy.value = false
  }
}

const copied = ref('')
let copyTimer: ReturnType<typeof setTimeout> | undefined
async function copy(env: string, url: string) {
  await copyText(href(url))
  copied.value = env
  clearTimeout(copyTimer)
  copyTimer = setTimeout(() => (copied.value = ''), 1500)
}
onBeforeUnmount(() => clearTimeout(copyTimer))
</script>

<template>
  <div class="min-h-dvh">
    <SiteHeader />
    <NoAccess v-if="noAccess" :info="noAccess" />
    <NotFoundView v-else-if="notFound" :app-id="id" embedded />
    <main v-else class="mx-auto flex max-w-[1440px] flex-col gap-5 px-4 pt-5 pb-12 sm:px-8 xl:px-10">
      <RouterLink
        to="/"
        class="inline-flex min-h-9 items-center gap-1.5 self-start text-sm font-medium text-accent no-underline hover:text-accent-ink"
      >
        <Icon name="back" :size="16" />
        All apps
      </RouterLink>
      <p v-if="toast" role="status" class="status-ok">{{ toast }}</p>

      <div v-if="loading && !app" aria-busy="true" aria-label="Loading app" class="flex items-center gap-[18px]">
        <span class="size-16 animate-pulse rounded-[14px] bg-[#e9ebee]" />
        <span class="flex flex-col gap-2.5">
          <span class="h-6 w-56 animate-pulse rounded bg-[#e9ebee]" />
          <span class="h-3.5 w-96 max-w-[60vw] animate-pulse rounded bg-[#e9ebee]" />
        </span>
      </div>

      <div
        v-if="error"
        role="alert"
        class="flex flex-wrap items-center gap-x-3.5 gap-y-2 rounded-xl border border-warn-line bg-warn-soft px-5 py-4 text-warn"
      >
        <Icon name="info" :size="20" />
        <div class="flex min-w-0 flex-[1_1_280px] flex-col gap-0.5">
          <span class="text-[15px] font-semibold">{{ app ? 'Something went wrong' : "Couldn't load this app" }}</span>
          <span class="text-[13.5px]">{{ error }}</span>
        </div>
        <button v-if="!app" type="button" class="btn btn-secondary" @click="load">Try again</button>
      </div>

      <template v-if="app">
        <div class="flex flex-wrap items-center justify-between gap-x-8 gap-y-4">
          <div class="flex min-w-0 max-w-[820px] items-center gap-[18px]">
            <AppIcon :app="app" :size="64" />
            <div class="flex min-w-0 flex-col gap-1.5">
              <h1 class="m-0 text-[32px] leading-tight font-semibold">{{ app.name }}</h1>
              <p v-if="app.description" class="m-0 text-base leading-normal text-muted">{{ app.description }}</p>
            </div>
          </div>
          <div class="flex flex-wrap gap-2.5">
            <button type="button" :aria-pressed="app.favorite" class="btn btn-secondary" :disabled="favBusy" @click="toggleFav">
              <Icon name="star" :size="20" :filled="app.favorite" :class="app.favorite ? 'text-star' : 'text-faint'" />
              {{ app.favorite ? 'Favorite' : 'Add to favorites' }}
            </button>
            <RouterLink v-if="app.can_edit" :to="editTo" class="btn btn-secondary">Edit</RouterLink>
            <a v-if="openURL" :href="openURL" target="_blank" rel="noopener" class="btn btn-primary px-5" @click="trackOpen(app.id)">
              Open app
              <Icon name="external" :size="16" />
              <span class="sr-only">(opens in a new tab)</span>
            </a>
          </div>
        </div>

        <section
          v-if="showHealth"
          aria-label="Health"
          class="flex flex-wrap items-center gap-x-7 gap-y-2 rounded-xl border px-5 py-3.5 text-[13px]"
          :class="status && !status.up ? 'border-warn-line bg-warn-soft' : 'border-line bg-white'"
        >
          <template v-if="status">
            <span class="inline-flex items-center gap-2">
              <span
                class="size-2.5 rounded-full"
                :class="status.up ? 'bg-ok shadow-[0_0_0_4px_var(--color-ok-soft)]' : 'bg-down'"
              />
              <span class="text-[15px] font-semibold" :class="status.up ? 'text-ok' : 'text-down'">{{ status.up ? 'Up' : 'Down' }}</span>
            </span>
            <span :class="status.up ? 'text-subtle' : 'text-warn'">
              {{ status.up ? 'Response' : 'Error' }}
              <span class="font-medium" :class="status.up ? 'text-ink' : 'text-warn-ink'">
                {{ status.up ? `HTTP ${status.http_status}, ${status.latency_ms} ms` : status.error || `HTTP ${status.http_status}` }}
              </span>
            </span>
            <span :class="status.up ? 'text-subtle' : 'text-warn'" :title="new Date(status.since).toLocaleString()">
              {{ status.up ? 'Up since' : 'Down since' }}
              <span class="font-medium" :class="status.up ? 'text-ink' : 'text-warn-ink'">
                {{ status.up ? when(status.since) : timeAgo(status.since).toLowerCase() }}
              </span>
            </span>
            <span :class="status.up ? 'text-subtle' : 'text-warn'" :title="new Date(status.checked_at).toLocaleString()">
              Last check
              <span class="font-medium" :class="status.up ? 'text-ink' : 'text-warn-ink'">{{ timeAgo(status.checked_at).toLowerCase() }}</span>
            </span>
          </template>
          <span v-else class="inline-flex items-center gap-2 text-subtle">
            <span class="size-2.5 rounded-full bg-disabled" />
            Not checked yet. The portal checks every minute.
          </span>
          <span
            v-if="status?.target || app.health_url"
            class="ml-auto min-w-0 truncate font-mono text-xs"
            :class="status && !status.up ? 'text-warn' : 'text-faint'"
          >
            {{ displayURL(status?.target || app.health_url) }}
          </span>
        </section>

        <div class="flex flex-col gap-5 lg:flex-row lg:items-start">
          <div class="flex min-w-0 flex-1 flex-col gap-5">
            <section v-if="app.access" aria-labelledby="access-h" class="flex gap-3.5 rounded-xl bg-accent-soft px-5 py-[18px] text-accent-ink">
              <Icon name="info" :size="20" class="mt-px" />
              <div class="flex min-w-0 flex-col gap-1">
                <h2 id="access-h" class="m-0 text-[15px] font-semibold">Getting access</h2>
                <p class="m-0 text-sm leading-normal whitespace-pre-line">{{ app.access }}</p>
              </div>
            </section>

            <section v-if="urls.length" aria-labelledby="env-h" class="card px-6 pt-5 pb-2">
              <h2 id="env-h" class="section-title mb-2">Environments</h2>
              <ul class="m-0 list-none p-0">
                <li
                  v-for="[env, url] in urls"
                  :key="env"
                  class="flex flex-wrap items-center gap-x-4 gap-y-1.5 border-t border-line-soft py-3 first:border-t-0"
                >
                  <span class="flex w-[180px] shrink-0 items-center gap-2 text-sm font-medium">
                    {{ envLabel(env) }}
                    <span v-if="url === primary" class="chip-blue px-1.5 py-px text-[11.5px]">Open app</span>
                  </span>
                  <span class="min-w-0 flex-[1_1_240px] font-mono text-[13px] break-all">{{ displayURL(url) }}</span>
                  <span class="flex gap-1.5">
                    <button
                      type="button"
                      class="icon-btn size-[34px] border border-line bg-white"
                      :aria-label="copied === env ? `Copied ${envLabel(env)} URL` : `Copy ${envLabel(env)} URL`"
                      :title="copied === env ? 'Copied' : 'Copy URL'"
                      @click="copy(env, url)"
                    >
                      <Icon :name="copied === env ? 'check' : 'copy'" :size="16" :class="{ 'text-ok': copied === env }" />
                    </button>
                    <a
                      :href="href(url)"
                      target="_blank"
                      rel="noopener"
                      class="icon-btn size-[34px] border border-line bg-white"
                      @click="trackOpen(app.id)"
                      :aria-label="`Open ${envLabel(env)}`"
                      title="Open"
                    >
                      <Icon name="external" :size="16" />
                    </a>
                  </span>
                </li>
              </ul>
              <p class="sr-only" role="status">{{ copied ? `Copied ${envLabel(copied)} URL` : '' }}</p>
            </section>
          </div>

          <aside class="flex flex-col gap-5 lg:w-[380px] lg:shrink-0">
            <section aria-labelledby="about-h" class="card flex flex-col gap-4 px-6 py-5">
              <h2 id="about-h" class="section-title">About</h2>
              <dl class="m-0 grid grid-cols-[96px_1fr] items-center gap-x-4 gap-y-3 text-sm">
                <dt class="text-subtle">Owner team</dt>
                <dd class="m-0">
                  <RouterLink v-if="app.owner" :to="`/teams/${encodeURIComponent(app.owner)}`" class="link font-normal">
                    {{ app.owner }}
                  </RouterLink>
                  <span v-else class="text-subtle">No owner team</span>
                </dd>
                <template v-if="app.contact">
                  <dt class="text-subtle">Contact</dt>
                  <dd class="m-0 break-words">
                    <a v-if="isEmail(app.contact)" :href="`mailto:${app.contact}`" class="link font-normal">{{ app.contact }}</a>
                    <span v-else>{{ app.contact }}</span>
                  </dd>
                </template>
                <template v-if="app.category">
                  <dt class="text-subtle">Category</dt>
                  <dd class="m-0">{{ optionLabel(settings?.categories, app.category) }}</dd>
                </template>
                <template v-if="app.runs_on">
                  <dt class="text-subtle">Runs on</dt>
                  <dd class="m-0">{{ optionLabel(settings?.runs_on, app.runs_on) }}</dd>
                </template>
                <template v-if="app.version">
                  <dt class="text-subtle">Version</dt>
                  <dd class="m-0 font-mono text-[13px]">{{ app.version }}</dd>
                </template>
                <dt class="text-subtle">Visible to</dt>
                <dd class="m-0 flex flex-wrap items-center gap-1.5">
                  <template v-if="app.visibility === 'everyone'">
                    <Icon name="globe" :size="14" class="text-faint" />
                    Everyone
                  </template>
                  <template v-else-if="app.groups.length">
                    <Icon name="group" :size="14" class="text-faint" />
                    <span v-for="g in app.groups" :key="g" class="chip-group">{{ g }}</span>
                  </template>
                  <template v-else>
                    <Icon name="lock" :size="14" class="text-faint" />
                    Admins only
                  </template>
                </dd>
                <dt class="text-subtle">App ID</dt>
                <dd class="m-0 font-mono text-[13px] break-all">{{ app.id }}</dd>
              </dl>
            </section>

            <section v-if="links.length" aria-labelledby="links-h" class="card flex flex-col gap-1 px-4 pt-5 pb-3">
              <h2 id="links-h" class="section-title mb-1.5 px-2">Links</h2>
              <a
                v-for="l in links"
                :key="l.key"
                :href="href(l.url)"
                target="_blank"
                rel="noopener"
                class="flex items-center gap-3 rounded-lg p-2 text-ink no-underline hover:bg-hover hover:text-ink"
              >
                <span class="flex size-8 shrink-0 items-center justify-center rounded-lg bg-chip text-muted">
                  <Icon :name="linkIcon[l.key]" :size="16" />
                </span>
                <span class="flex min-w-0 flex-1 flex-col">
                  <span class="text-sm font-semibold">{{ l.label }}</span>
                  <span class="truncate text-[12.5px] text-subtle">{{ hostOf(l.url) }}</span>
                </span>
                <Icon name="external" :size="14" class="text-faint" />
              </a>
            </section>

            <p class="m-0 px-2 text-[12.5px] leading-normal text-subtle">
              <template v-if="app.created_by">Added by {{ app.created_by }}. </template>
              Last updated {{ fmtDate(app.updated_at) }}<template v-if="app.updated_by"> by {{ app.updated_by }}</template>.
            </p>
          </aside>
        </div>

        <section v-if="related.length" aria-labelledby="more-h" class="flex flex-col gap-3 pt-3">
          <h2 id="more-h" class="m-0 text-lg font-semibold">More in {{ optionLabel(settings?.categories, app.category) }}</h2>
          <ul class="m-0 grid list-none gap-3 p-0 sm:grid-cols-2 lg:grid-cols-3">
            <li v-for="r in related" :key="r.id" class="min-w-0">
              <RouterLink
                :to="`/apps/${encodeURIComponent(r.id)}`"
                class="flex items-center gap-3 rounded-xl border border-line bg-white px-3.5 py-3 text-ink no-underline transition-[border-color,box-shadow] hover:border-field hover:text-ink hover:shadow-[0_4px_14px_rgba(21,23,28,0.08)]"
              >
                <AppIcon :app="r" :size="32" />
                <span class="flex min-w-0 flex-col">
                  <span class="truncate text-sm font-semibold">{{ r.name }}</span>
                  <span v-if="r.description" class="truncate text-[13px] text-subtle">{{ r.description }}</span>
                </span>
              </RouterLink>
            </li>
          </ul>
        </section>
      </template>
    </main>
  </div>
</template>
