<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import AppIcon from '@/components/AppIcon.vue'
import Icon from '@/components/Icon.vue'
import { type Group, type Visibility, api, errorText, plural } from '@/api'

const props = defineProps<{
  app: { id: string; name: string; visibility: Visibility; groups: string[]; icon_color: string; has_icon?: boolean; icon_version?: number }
  // when given, the dialog can count the people who lose the app
  groups?: Group[]
}>()
const emit = defineEmits<{ close: []; deleted: [] }>()

const confirm = ref('')
const busy = ref(false)
const error = ref('')
const input = ref<HTMLInputElement>()

const where = computed(() =>
  props.app.visibility === 'groups' && props.app.groups.length
    ? `Removed from the catalog and from ${props.app.groups.join(', ')}`
    : 'Removed from the catalog',
)
const reach = computed(() => {
  const a = props.app
  if (a.visibility === 'everyone') return 'Every signed-in user will no longer see it'
  if (!a.groups.length) return 'Only admins could see it'
  const names = a.groups.join(', ')
  if (!props.groups) return `Members of ${names} will no longer see it`
  const people = new Set(props.groups.filter((g) => a.groups.includes(g.name)).flatMap((g) => g.members.map((m) => m.subject)))
  return `${plural(people.size, 'person', 'people')} in ${names} will no longer see it`
})
const canDelete = computed(() => confirm.value.trim() === props.app.id && !busy.value)

async function doDelete() {
  if (!canDelete.value) return
  busy.value = true
  error.value = ''
  try {
    await api.deleteApp(props.app.id)
    emit('deleted')
  } catch (e) {
    error.value = errorText(e, 'Could not delete the app. Try again.')
  } finally {
    busy.value = false
  }
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}
onMounted(async () => {
  window.addEventListener('keydown', onKey)
  await nextTick()
  input.value?.focus()
})
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <Teleport to="body">
    <div
      class="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-ink/50 px-4 pt-[12vh] pb-4"
      @click.self="emit('close')"
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="del-title"
        class="flex w-full max-w-[480px] flex-col gap-[18px] rounded-[14px] bg-white p-7 shadow-[0_20px_50px_rgba(21,23,28,0.25)]"
      >
        <div class="flex items-center gap-3.5">
          <span class="relative shrink-0">
            <AppIcon :app="app" :size="44" />
            <span
              class="absolute -right-1.5 -bottom-1.5 flex size-6 items-center justify-center rounded-full bg-warn-soft text-warn ring-2 ring-white"
            >
              <Icon name="trash" :size="13" />
            </span>
          </span>
          <div class="flex flex-col gap-0.5">
            <h2 id="del-title" class="m-0 text-xl font-semibold">Delete {{ app.name }}?</h2>
            <p class="m-0 text-[13px] text-subtle">This can't be undone.</p>
          </div>
        </div>
        <ul class="m-0 flex list-none flex-col gap-2 rounded-[10px] border border-line-soft bg-panel px-3.5 py-3 text-[13.5px] text-muted">
          <li class="flex items-center gap-2.5"><span class="size-2 shrink-0 rounded-full bg-warn" />{{ where }}</li>
          <li class="flex items-center gap-2.5"><span class="size-2 shrink-0 rounded-full bg-warn" />{{ reach }}</li>
          <li class="flex items-center gap-2.5"><span class="size-2 shrink-0 rounded-full bg-ok" />{{ app.name }} itself keeps running</li>
        </ul>
        <form class="flex flex-col gap-[18px]" @submit.prevent="doDelete">
          <div class="flex flex-col gap-1.5">
            <label for="del-confirm" class="label">
              Type <span class="rounded bg-chip px-1.5 py-px font-mono">{{ app.id }}</span> to confirm
            </label>
            <input
              id="del-confirm"
              ref="input"
              v-model="confirm"
              type="text"
              autocomplete="off"
              spellcheck="false"
              class="field font-mono"
            />
          </div>
          <p v-if="error" role="alert" class="status-error">{{ error }}</p>
          <div class="flex flex-wrap justify-end gap-2.5">
            <button type="button" class="btn btn-secondary" @click="emit('close')">Cancel</button>
            <button type="submit" class="btn btn-danger" :disabled="!canDelete">
              {{ busy ? 'Deleting...' : 'Delete app' }}
            </button>
          </div>
        </form>
      </div>
    </div>
  </Teleport>
</template>
