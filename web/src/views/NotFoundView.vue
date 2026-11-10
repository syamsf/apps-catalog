<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import AppIcon from '@/components/AppIcon.vue'
import Icon from '@/components/Icon.vue'
import SiteHeader from '@/components/SiteHeader.vue'
import { type App, api, displayURL, primaryURL, similarApps } from '@/api'
import { me, settings } from '@/session'

const props = defineProps<{ appId?: string; embedded?: boolean }>()
const router = useRouter()

// "jenkins-old" reads better as "jenkins old" in a search box
const q = ref((props.appId ?? '').replace(/[-_]+/g, ' ').trim())
const apps = ref<App[]>([])
watch(
  () => props.appId,
  async (id) => {
    if (!id) return
    try {
      apps.value = (await api.apps()).apps
    } catch {
      apps.value = []
    }
  },
  { immediate: true },
)
const matches = computed(() => similarApps(apps.value, q.value))
const envs = computed(() => settings.value?.environments ?? [])

function search() {
  router.push({ name: 'catalog', query: q.value.trim() ? { q: q.value.trim() } : {} })
}
</script>

<template>
  <div class="min-h-dvh">
    <SiteHeader v-if="!embedded && me" />
    <main class="mx-auto max-w-[560px] px-4 py-16">
      <div class="card flex flex-col gap-4 rounded-2xl p-9">
        <h1 class="m-0 text-2xl font-semibold">{{ appId ? 'App not found' : 'Page not found' }}</h1>
        <p class="m-0 text-[15px] leading-relaxed text-muted">
          <template v-if="appId">
            There is no app with the ID <span class="font-mono text-ink">{{ appId }}</span>. It may have been renamed or
            removed.
          </template>
          <template v-else>This page doesn't exist. Check the address, or go back to the catalog.</template>
        </p>
        <template v-if="appId">
          <form role="search" class="relative" @submit.prevent="search">
            <label for="nf-q" class="sr-only">Search the catalog</label>
            <Icon name="search" class="pointer-events-none absolute top-[13px] left-3.5 text-subtle" />
            <input id="nf-q" v-model="q" type="search" class="field h-11 pl-[42px] text-[15px]" />
          </form>
          <div v-if="matches.length" class="-mt-1 flex flex-col gap-0.5">
            <span class="text-xs font-semibold text-subtle">Close matches</span>
            <RouterLink
              v-for="a in matches"
              :key="a.id"
              :to="`/apps/${encodeURIComponent(a.id)}`"
              class="flex items-center gap-3 rounded-lg p-2 text-ink no-underline hover:bg-hover hover:text-ink"
            >
              <AppIcon :app="a" :size="32" />
              <span class="flex min-w-0 flex-1 flex-col">
                <span class="truncate text-sm font-semibold">{{ a.name }}</span>
                <span v-if="primaryURL(a, envs)" class="truncate font-mono text-xs text-subtle">
                  {{ displayURL(primaryURL(a, envs)!) }}
                </span>
              </span>
              <Icon name="arrow" :size="16" class="text-faint" />
            </RouterLink>
          </div>
        </template>
        <div>
          <RouterLink to="/" class="btn btn-secondary">Back to catalog</RouterLink>
        </div>
      </div>
    </main>
  </div>
</template>
