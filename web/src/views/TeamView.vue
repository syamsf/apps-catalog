<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import AppCard from '@/components/AppCard.vue'
import Avatar from '@/components/Avatar.vue'
import Icon from '@/components/Icon.vue'
import SiteHeader from '@/components/SiteHeader.vue'
import NotFoundView from '@/views/NotFoundView.vue'
import { type App, ApiError, type Team, api, copyText, errorText, isAbort } from '@/api'
import { optionLabel, setTitle, settings } from '@/session'

const props = defineProps<{ name: string }>()

const team = ref<Team>()
const notFound = ref(false)
const error = ref('')
const loading = ref(true)

let ctrl: AbortController | undefined
async function load() {
  ctrl?.abort()
  const c = (ctrl = new AbortController())
  team.value = undefined
  notFound.value = false
  error.value = ''
  loading.value = true
  try {
    team.value = await api.team(props.name, c.signal)
    setTitle(team.value.name)
  } catch (e) {
    if (isAbort(e)) return
    if (e instanceof ApiError && e.status === 404) {
      notFound.value = true
      setTitle('Page not found')
    } else error.value = errorText(e, 'Check your connection and try again.')
  } finally {
    if (ctrl === c) loading.value = false
  }
}
watch(() => props.name, load, { immediate: true })
onBeforeUnmount(() => ctrl?.abort())

const envs = computed(() => settings.value?.environments ?? [])
const members = computed(() => team.value?.members ?? [])
const apps = computed(() => team.value?.apps ?? [])

const favBusy = ref(new Set<string>())
async function toggleFav(app: App) {
  if (favBusy.value.has(app.id)) return
  favBusy.value.add(app.id)
  app.favorite = !app.favorite
  try {
    await api.favorite(app.id, app.favorite)
  } catch (e) {
    app.favorite = !app.favorite
    error.value = errorText(e, 'Could not update favorites.')
  } finally {
    favBusy.value.delete(app.id)
  }
}

const notice = ref('')
let noticeTimer: ReturnType<typeof setTimeout> | undefined
async function copyURL(app: App, url: string) {
  await copyText(url)
  notice.value = `Copied the URL of ${app.name}.`
  clearTimeout(noticeTimer)
  noticeTimer = setTimeout(() => (notice.value = ''), 2500)
}
onBeforeUnmount(() => clearTimeout(noticeTimer))
</script>

<template>
  <div class="min-h-dvh">
    <SiteHeader />
    <NotFoundView v-if="notFound" embedded />
    <main v-else class="mx-auto flex max-w-[1440px] flex-col gap-5 px-4 pt-5 pb-12 sm:px-8 xl:px-10">
      <RouterLink
        to="/"
        class="inline-flex min-h-9 items-center gap-1.5 self-start text-sm font-medium text-accent no-underline hover:text-accent-ink"
      >
        <Icon name="back" :size="16" />
        All apps
      </RouterLink>

      <div v-if="loading && !team" aria-busy="true" aria-label="Loading team" class="flex flex-col gap-2.5">
        <span class="h-7 w-56 animate-pulse rounded bg-[#e9ebee]" />
        <span class="h-3.5 w-96 max-w-[60vw] animate-pulse rounded bg-[#e9ebee]" />
      </div>

      <div
        v-if="error"
        role="alert"
        class="flex flex-wrap items-center gap-x-3.5 gap-y-2 rounded-xl border border-warn-line bg-warn-soft px-5 py-4 text-warn"
      >
        <Icon name="info" :size="20" />
        <div class="flex min-w-0 flex-[1_1_280px] flex-col gap-0.5">
          <span class="text-[15px] font-semibold">{{ team ? 'Something went wrong' : "Couldn't load this team" }}</span>
          <span class="text-[13.5px]">{{ error }}</span>
        </div>
        <button v-if="!team" type="button" class="btn btn-secondary" @click="load">Try again</button>
      </div>

      <template v-if="team">
        <div class="flex flex-col gap-1.5">
          <span class="eyebrow">Team</span>
          <h1 class="m-0 font-mono text-[28px] leading-tight font-semibold">{{ team.name }}</h1>
          <p v-if="team.description" class="m-0 text-base leading-normal text-muted">{{ team.description }}</p>
        </div>

        <section aria-labelledby="members-h" class="card flex flex-col gap-3 px-6 py-5">
          <h2 id="members-h" class="section-title">Members</h2>
          <p v-if="!members.length" class="m-0 text-sm text-subtle">No members yet.</p>
          <ul v-else class="m-0 flex list-none flex-wrap gap-x-5 gap-y-2.5 p-0">
            <li v-for="(m, i) in members" :key="i" class="flex items-center gap-2 text-sm">
              <Avatar :name="m" :size="28" />
              {{ m }}
            </li>
          </ul>
        </section>

        <section aria-labelledby="apps-h" class="flex flex-col gap-3">
          <h2 id="apps-h" class="m-0 text-lg font-semibold">
            Apps it owns <span class="font-mono text-sm font-normal text-subtle">{{ apps.length }}</span>
          </h2>
          <p v-if="!apps.length" class="m-0 text-sm text-subtle">No apps you can see.</p>
          <div v-else class="grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-4">
            <AppCard
              v-for="app in apps"
              :key="app.id"
              :app="app"
              :envs="envs"
              :category="optionLabel(settings?.categories, app.category)"
              :runs-on="optionLabel(settings?.runs_on, app.runs_on)"
              :admin="false"
              :fav-busy="favBusy.has(app.id)"
              hide-status
              @favorite="toggleFav(app)"
              @copy="(url) => copyURL(app, url)"
            />
          </div>
        </section>
      </template>
    </main>

    <p
      v-if="notice"
      role="status"
      class="fixed bottom-5 left-1/2 z-50 m-0 -translate-x-1/2 rounded-lg bg-ink px-4 py-2.5 text-sm font-medium text-white shadow-lg"
    >
      {{ notice }}
    </p>
  </div>
</template>
