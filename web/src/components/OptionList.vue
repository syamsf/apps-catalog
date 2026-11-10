<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import Icon from '@/components/Icon.vue'
import { type Option, type OptionKind, type Portal, api, errorText, plural } from '@/api'

const props = defineProps<{
  kind: OptionKind
  title: string
  help: string
  addLabel: string
  items: Option[]
  // drag to reorder
  ordered?: boolean
  // key that gets a tag, e.g. the environment open app uses
  tagKey?: string
  tag?: string
}>()
const emit = defineEmits<{ saved: [settings: Portal, what: string] }>()

const list = ref<Option[]>([...props.items])
watch(
  () => props.items,
  (v) => (list.value = [...v]),
)
const busy = ref(false)
const error = ref('')

async function run(fn: () => Promise<Portal>, what: string, fail: string) {
  busy.value = true
  error.value = ''
  try {
    emit('saved', await fn(), what)
    return true
  } catch (e) {
    error.value = errorText(e, fail)
    list.value = [...props.items]
    return false
  } finally {
    busy.value = false
  }
}

// add
const draft = ref('')
async function add() {
  const label = draft.value.trim()
  if (!label || busy.value) return
  if (await run(() => api.addOption(props.kind, label), `Added ${label}`, 'Could not add it.')) draft.value = ''
}

// rename
const renaming = ref('')
const renameText = ref('')
async function startRename(o: Option) {
  renaming.value = o.key
  renameText.value = o.label
  await nextTick()
  document.getElementById(`rn-${props.kind}-${o.key}`)?.focus()
}
async function saveRename(o: Option) {
  const label = renameText.value.trim()
  if (!label || label === o.label) {
    renaming.value = ''
    return
  }
  if (await run(() => api.renameOption(props.kind, o.key, label), `Renamed ${o.label} to ${label}`, 'Could not rename it.'))
    renaming.value = ''
}

function remove(o: Option) {
  if (o.count || busy.value) return
  run(() => api.removeOption(props.kind, o.key), `Removed ${o.label}`, 'Could not remove it.')
}

// reorder by drag, or with the arrow keys on the handle
const dragging = ref('')
function onDragOver(o: Option) {
  if (!dragging.value || dragging.value === o.key) return
  const from = list.value.findIndex((x) => x.key === dragging.value)
  const to = list.value.findIndex((x) => x.key === o.key)
  const next = [...list.value]
  next.splice(to, 0, next.splice(from, 1)[0]!)
  list.value = next
}
function saveOrder(moved: Option) {
  const keys = list.value.map((x) => x.key)
  if (keys.join() === props.items.map((x) => x.key).join()) return
  run(() => api.reorderOptions(props.kind, keys), `Moved ${moved.label}`, 'Could not save the order.')
}
function onDragEnd() {
  const moved = list.value.find((x) => x.key === dragging.value)
  dragging.value = ''
  if (moved) saveOrder(moved)
}
async function onHandleKey(e: KeyboardEvent, o: Option) {
  if (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return
  e.preventDefault()
  const i = list.value.findIndex((x) => x.key === o.key)
  const j = e.key === 'ArrowUp' ? i - 1 : i + 1
  if (j < 0 || j >= list.value.length) return
  const next = [...list.value]
  next.splice(j, 0, next.splice(i, 1)[0]!)
  list.value = next
  saveOrder(o)
  await nextTick()
  document.getElementById(`mv-${props.kind}-${o.key}`)?.focus()
}
</script>

<template>
  <section :aria-labelledby="`ol-${kind}`" class="card flex flex-col gap-3 px-5 pt-5 pb-4">
    <div class="flex flex-col gap-1">
      <h2 :id="`ol-${kind}`" class="section-title text-[17px]">{{ title }}</h2>
      <p class="m-0 text-[13px] leading-normal text-subtle">{{ help }}</p>
    </div>
    <ul class="m-0 flex list-none flex-col gap-0.5 p-0" :aria-busy="busy">
      <li
        v-for="o in list"
        :key="o.key"
        class="flex min-h-10 items-center gap-2 rounded-lg pr-1"
        :class="[ordered ? 'pl-0.5' : 'pl-2.5', dragging === o.key ? 'bg-accent-soft' : 'hover:bg-hover']"
        :draggable="ordered && renaming !== o.key"
        @dragstart="dragging = o.key"
        @dragover.prevent="onDragOver(o)"
        @dragend="onDragEnd"
      >
        <button
          v-if="ordered"
          :id="`mv-${kind}-${o.key}`"
          type="button"
          class="icon-btn size-7 cursor-grab"
          :aria-label="`Move ${o.label}, use the up and down arrow keys`"
          @keydown="onHandleKey($event, o)"
        >
          <Icon name="grip" :size="16" />
        </button>
        <template v-if="renaming === o.key">
          <form class="flex min-w-0 flex-1 items-center gap-1.5" @submit.prevent="saveRename(o)">
            <label :for="`rn-${kind}-${o.key}`" class="sr-only">New name for {{ o.label }}</label>
            <input :id="`rn-${kind}-${o.key}`" v-model="renameText" type="text" class="field h-8 text-sm" @keydown.esc="renaming = ''" />
            <button type="submit" class="btn btn-primary btn-sm min-h-8 px-3" :disabled="busy">Save</button>
            <button type="button" class="btn btn-ghost btn-sm min-h-8 px-2.5" @click="renaming = ''">Cancel</button>
          </form>
        </template>
        <template v-else>
          <span class="min-w-0 truncate text-sm font-medium">{{ o.label }}</span>
          <span v-if="tag && o.key === tagKey" class="chip-blue px-1.5 py-px text-[11px]">{{ tag }}</span>
          <span class="ml-auto shrink-0 text-[12.5px]" :class="o.count ? 'text-subtle' : 'text-faint'">
            {{ o.count ? plural(o.count, 'app') : 'Unused' }}
          </span>
          <button type="button" class="icon-btn size-7" :aria-label="`Rename ${o.label}`" @click="startRename(o)">
            <Icon name="pencil" :size="14" />
          </button>
          <button
            type="button"
            class="icon-btn size-7"
            :class="{ invisible: o.count > 0 }"
            :aria-label="`Remove ${o.label}`"
            :disabled="o.count > 0 || busy"
            @click="remove(o)"
          >
            <Icon name="x" :size="14" />
          </button>
        </template>
      </li>
      <li v-if="!list.length" class="px-2.5 py-2 text-sm text-subtle">None yet.</li>
    </ul>
    <form class="relative" @submit.prevent="add">
      <label :for="`add-${kind}`" class="sr-only">{{ addLabel }}</label>
      <Icon name="plus" :size="16" class="pointer-events-none absolute top-3 left-3 text-accent" />
      <input
        :id="`add-${kind}`"
        v-model="draft"
        type="text"
        :placeholder="addLabel"
        class="field h-10 border-dashed pl-9 text-sm focus:border-solid"
        :disabled="busy"
      />
      <button v-if="draft.trim()" type="submit" class="btn btn-primary btn-sm absolute top-1 right-1 min-h-8 px-3" :disabled="busy">
        Add
      </button>
    </form>
    <p v-if="error" role="alert" class="status-error">{{ error }}</p>
  </section>
</template>
