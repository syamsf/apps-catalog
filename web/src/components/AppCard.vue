<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import AppIcon from '@/components/AppIcon.vue'
import Icon from '@/components/Icon.vue'
import Popover from '@/components/Popover.vue'
import { type App, type Option, displayURL, href, primaryURL, statusTitle, trackOpen } from '@/api'

const props = defineProps<{
  app: App
  envs: Option[]
  category: string
  runsOn: string
  admin: boolean
  favBusy?: boolean
  hideStatus?: boolean
  // highlighted from the search box
  active?: boolean
}>()
const emit = defineEmits<{ favorite: []; copy: [url: string]; delete: [] }>()

const url = computed(() => primaryURL(props.app, props.envs) ?? '')
const openURL = computed(() => (url.value ? href(url.value) : ''))
const detailsTo = computed(() => `/apps/${encodeURIComponent(props.app.id)}`)

const vis = computed(() => {
  const a = props.app
  if (a.visibility === 'everyone') return { icon: 'globe' as const, text: 'Everyone' }
  if (!a.groups.length) return { icon: 'lock' as const, text: 'Admins only' }
  return { icon: 'group' as const, text: a.groups.join(', ') }
})
const hasMenu = computed(() => props.admin || !!openURL.value)
</script>

<template>
  <article
    class="group relative flex flex-col rounded-xl border bg-white transition-[border-color,box-shadow] hover:border-field hover:shadow-[0_4px_14px_rgba(21,23,28,0.08)]"
    :class="active ? 'border-field shadow-[0_4px_14px_rgba(21,23,28,0.08)]' : 'border-line'"
  >
    <span v-if="active" aria-hidden="true" class="absolute top-3 bottom-3 -left-px w-[3px] rounded-r-full bg-accent" />
    <div class="flex flex-1 flex-col gap-2.5 pt-4 pr-3 pb-3.5 pl-4">
      <div class="flex items-start gap-3">
        <AppIcon :app="app" :size="44" />
        <div class="flex min-w-0 flex-1 flex-col gap-[3px]">
          <h3 class="m-0 text-base leading-snug font-semibold">
            <!-- the link covers the whole card, buttons sit above it -->
            <component
              :is="openURL ? 'a' : RouterLink"
              :href="openURL || undefined"
              :to="openURL ? undefined : detailsTo"
              :target="openURL ? '_blank' : undefined"
              :rel="openURL ? 'noopener' : undefined"
              @click="openURL && trackOpen(app.id)"
              class="inline-flex max-w-full items-center gap-1.5 text-ink no-underline group-hover:text-accent-ink after:absolute after:inset-0 after:rounded-xl after:content-['']"
            >
              <span class="truncate">{{ app.name }}</span>
              <Icon v-if="openURL" name="external" :size="14" class="opacity-0 transition-opacity group-hover:opacity-100" />
              <span v-if="openURL" class="sr-only">(opens in a new tab)</span>
            </component>
          </h3>
          <span v-if="url" class="truncate font-mono text-xs text-subtle">{{ displayURL(url) }}</span>
        </div>
        <div class="relative z-10 -mt-1 flex items-center focus-within:z-30">
          <button
            type="button"
            :aria-pressed="app.favorite"
            :aria-label="app.favorite ? `Remove ${app.name} from favorites` : `Add ${app.name} to favorites`"
            :disabled="favBusy"
            class="icon-btn"
            :class="app.favorite ? 'text-star hover:text-star' : 'text-faint'"
            @click="emit('favorite')"
          >
            <Icon name="star" :size="18" :filled="app.favorite" />
          </button>
          <Popover
            v-if="hasMenu"
            :label="`More for ${app.name}`"
            align="right"
            panel-class="w-48"
            button-class="icon-btn aria-expanded:bg-chip aria-expanded:text-ink"
          >
            <template #trigger>
              <Icon name="more" :size="18" />
            </template>
            <template #default="{ close }">
              <RouterLink
                v-if="admin"
                :to="`/admin/apps/${encodeURIComponent(app.id)}/edit`"
                role="menuitem"
                class="menu-item"
              >
                <Icon name="pencil" :size="16" class="text-subtle" />
                Edit app
              </RouterLink>
              <button v-if="openURL" type="button" role="menuitem" class="menu-item" @click="(emit('copy', openURL), close())">
                <Icon name="copy" :size="16" class="text-subtle" />
                Copy URL
              </button>
              <template v-if="admin">
                <div class="menu-sep" />
                <button type="button" role="menuitem" class="menu-item text-warn hover:text-warn" @click="(close(false), emit('delete'))">
                  <Icon name="trash" :size="16" />
                  Delete
                </button>
              </template>
            </template>
          </Popover>
        </div>
      </div>
      <p v-if="app.description" class="m-0 line-clamp-2 text-sm leading-normal text-muted" :title="app.description">
        {{ app.description }}
      </p>
      <div v-if="category || runsOn" class="flex flex-wrap gap-1.5">
        <span v-if="category" class="chip">{{ category }}</span>
        <span v-if="runsOn" class="chip">{{ runsOn }}</span>
      </div>
    </div>
    <div class="flex min-h-11 items-center gap-3.5 border-t border-line-soft px-4 py-2 text-[13px]">
      <span v-if="!hideStatus" class="inline-flex shrink-0 items-center gap-1.5" :title="statusTitle(app.status)">
        <template v-if="!app.status">
          <span class="size-2 rounded-full bg-disabled" />
          <span class="text-faint">Not checked</span>
        </template>
        <template v-else-if="app.status.up">
          <span class="size-2 rounded-full bg-ok" />
          <span class="font-medium text-subtle">Up</span>
          <span class="font-mono text-xs text-faint">{{ app.status.latency_ms }} ms</span>
        </template>
        <template v-else>
          <span class="size-2 rounded-full bg-down" />
          <span class="font-medium text-down">Down</span>
        </template>
      </span>
      <span v-if="admin" class="inline-flex min-w-0 items-center gap-[5px] text-subtle" :title="`Visible to: ${vis.text}`">
        <Icon :name="vis.icon" :size="14" class="text-faint" />
        <span class="truncate">{{ vis.text }}</span>
      </span>
      <RouterLink
        :to="detailsTo"
        class="relative z-10 ml-auto inline-flex shrink-0 items-center gap-1 font-medium text-accent no-underline hover:text-accent-ink"
      >
        Details
        <Icon name="arrow" :size="14" />
        <span class="sr-only">about {{ app.name }}</span>
      </RouterLink>
    </div>
  </article>
</template>
