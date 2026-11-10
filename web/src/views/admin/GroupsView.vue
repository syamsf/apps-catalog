<script setup lang="ts">
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'
import Avatar from '@/components/Avatar.vue'
import Icon from '@/components/Icon.vue'
import { type App, type Group, api, errorText, plural } from '@/api'
import { flash } from '@/session'

const groups = ref<Group[]>([])
const apps = ref<App[]>([])
const loading = ref(true)
const error = ref('')
const toast = ref(flash.value && !flash.value.error ? flash.value.text : '')
flash.value = null
const query = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [g, a] = await Promise.all([api.groups(), api.apps()])
    groups.value = g.groups
    apps.value = a.apps
  } catch (e) {
    error.value = errorText(e, 'Could not load groups.')
  } finally {
    loading.value = false
  }
}
load()

const everyoneApps = computed(() => apps.value.filter((a) => a.visibility === 'everyone'))
const rows = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return groups.value
  return groups.value.filter(
    (g) => g.name.toLowerCase().includes(q) || g.description.toLowerCase().includes(q) || g.members.some((m) => m.name.toLowerCase().includes(q)),
  )
})
const cols = 'grid-cols-[minmax(220px,1.4fr)_minmax(200px,1fr)_minmax(220px,1.2fr)_20px]'
const groupLink = (g: Group) => `/admin/groups/${encodeURIComponent(g.name)}`
</script>

<template>
  <div class="flex flex-wrap items-end justify-between gap-x-6 gap-y-3">
    <div class="flex flex-col gap-1">
      <h1 class="m-0 text-[28px] font-semibold">Groups</h1>
      <p class="m-0 text-sm text-subtle">Everyone in a group sees that group's apps.</p>
    </div>
    <RouterLink to="/admin/groups/new" class="btn btn-primary px-[18px]">New group</RouterLink>
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
    <div class="relative max-w-[300px] flex-[1_1_220px]">
      <Icon name="search" :size="16" class="pointer-events-none absolute top-3 left-3 text-subtle" />
      <input v-model="query" type="search" aria-label="Search groups" placeholder="Search groups or people" class="field h-10 pl-9" />
    </div>
    <span class="ml-auto text-[13px] text-subtle">{{ plural(groups.length, 'group') }}</span>
  </div>

  <div class="card overflow-x-auto">
    <div class="min-w-[760px]">
      <div class="grid gap-5 border-b border-line px-5 py-2.5 text-xs font-semibold tracking-[0.04em] text-subtle uppercase" :class="cols">
        <span>Group</span><span>Members</span><span>Apps</span><span class="sr-only">Open</span>
      </div>

      <div class="grid items-center gap-5 border-b border-line-soft bg-[#fafcfa] px-5 py-3.5" :class="cols">
        <span class="flex flex-wrap items-center gap-2.5">
          <span class="chip-ok px-[9px] py-[3px] text-[13px]">Everyone</span>
          <span class="text-[13px] text-subtle">Built in</span>
        </span>
        <span class="text-[13px] text-muted">Every signed-in user</span>
        <span class="flex flex-wrap gap-1.5">
          <span v-for="a in everyoneApps" :key="a.id" class="chip">{{ a.name }}</span>
          <span v-if="!everyoneApps.length" class="text-[13px] text-subtle">No apps yet</span>
        </span>
        <span />
      </div>

      <template v-if="loading && !groups.length">
        <div v-for="i in 4" :key="i" class="grid items-center gap-5 border-b border-line-soft px-5 py-4 last:border-b-0" :class="cols">
          <span class="h-3.5 w-28 animate-pulse rounded bg-[#e9ebee]" />
          <span class="h-6 w-24 animate-pulse rounded-full bg-[#e9ebee]" />
          <span class="h-5 w-20 animate-pulse rounded bg-[#e9ebee]" />
          <span />
        </div>
      </template>

      <ul class="m-0 list-none p-0">
        <li
          v-for="g in rows"
          :key="g.name"
          class="relative grid items-center gap-5 border-b border-line-soft px-5 py-3.5 last:border-b-0 hover:bg-hover"
          :class="cols"
        >
          <span class="flex min-w-0 flex-col gap-0.5">
            <RouterLink
              :to="groupLink(g)"
              class="truncate font-mono text-[15px] font-medium text-ink no-underline after:absolute after:inset-0 after:content-[''] hover:text-ink"
            >
              {{ g.name }}
            </RouterLink>
            <span class="truncate text-[13px]" :class="g.description ? 'text-subtle' : 'text-faint'">
              {{ g.description || 'No description' }}
            </span>
          </span>
          <span class="flex items-center gap-2.5">
            <template v-if="g.members.length">
              <span class="flex">
                <Avatar
                  v-for="(m, i) in g.members.slice(0, 4)"
                  :key="m.subject"
                  :name="m.name"
                  :size="28"
                  class="ring-2 ring-white"
                  :class="{ '-ml-1.5': i > 0 }"
                />
              </span>
              <span class="text-[13px] text-muted">
                <template v-if="g.members.length > 4">+{{ g.members.length - 4 }}, </template>{{ plural(g.members.length, 'member') }}
              </span>
            </template>
            <span v-else class="chip-warn">No members</span>
          </span>
          <span class="flex flex-wrap gap-1.5">
            <span v-for="a in g.apps" :key="a.id" class="rounded-md border border-line px-[9px] py-[2px] text-[13px]">{{ a.name }}</span>
            <span v-if="!g.apps.length" class="chip-warn">No apps</span>
          </span>
          <Icon name="arrow" :size="16" class="text-faint" />
        </li>
      </ul>

      <div v-if="!loading && !rows.length" class="flex flex-col items-center gap-2 px-5 py-10 text-center">
        <template v-if="groups.length">
          <p class="m-0 text-base font-semibold">No groups match "{{ query.trim() }}"</p>
          <button type="button" class="btn btn-secondary" @click="query = ''">Clear search</button>
        </template>
        <template v-else>
          <p class="m-0 text-base font-semibold">No groups yet</p>
          <p class="m-0 text-sm text-subtle">Create a group to share apps with a set of people.</p>
          <RouterLink to="/admin/groups/new" class="btn btn-primary">New group</RouterLink>
        </template>
      </div>
    </div>
  </div>
</template>
