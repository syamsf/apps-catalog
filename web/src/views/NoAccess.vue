<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import AppIcon from '@/components/AppIcon.vue'
import Icon from '@/components/Icon.vue'
import { type AccessRequest, type NoAccessBody, api, copyText, errorText, fmtDate, timeAgo } from '@/api'

const props = defineProps<{ info: NoAccessBody }>()

const name = computed(() => props.info.app.name)
const groups = computed(() => props.info.groups)

const request = ref<AccessRequest | null>(props.info.request)
const status = computed(() => request.value?.status ?? '')
// approved but still here means access was removed later, so they can ask again
const canAsk = computed(() => status.value !== 'pending')
const note = ref('')
const sending = ref(false)
const error = ref('')
watch(
  () => props.info,
  (i) => {
    request.value = i.request
    note.value = ''
    error.value = ''
  },
)
async function ask() {
  if (sending.value) return
  sending.value = true
  error.value = ''
  try {
    request.value = await api.requestAccess(props.info.app.id, note.value.trim())
    note.value = ''
  } catch (e) {
    error.value = errorText(e, 'Could not send the request. Try again.')
  } finally {
    sending.value = false
  }
}

const copied = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined
async function copyEmail() {
  await copyText(props.info.admin_email)
  copied.value = true
  clearTimeout(timer)
  timer = setTimeout(() => (copied.value = false), 1500)
}
onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <main class="mx-auto max-w-[560px] px-4 py-16">
    <div class="card flex flex-col gap-4 rounded-2xl p-9">
      <span class="relative self-start">
        <AppIcon :app="{ id: info.app.id, name: info.app.name, icon_color: '' }" :size="56" />
        <span
          class="absolute -right-2 -bottom-2 flex size-[26px] items-center justify-center rounded-full border border-line bg-white text-muted"
        >
          <Icon name="lock" :size="14" />
        </span>
      </span>
      <h1 class="m-0 text-2xl leading-snug font-semibold">You don't have access to {{ name }}</h1>
      <div class="flex flex-col gap-1.5 text-[15px] leading-relaxed text-muted">
        <p v-if="groups.length" class="m-0">
          {{ name }} is shared with
          <template v-for="(g, i) in groups" :key="g"
            ><span class="chip-group">{{ g }}</span
            ><template v-if="i < groups.length - 2">, </template
            ><template v-else-if="i === groups.length - 2"> and </template></template
          >. You aren't in {{ groups.length === 1 ? 'that group' : groups.length === 2 ? 'either group' : 'any of them' }}.
        </p>
        <p v-else class="m-0">{{ name }} isn't shared with any group yet.</p>
        <p class="m-0">Request access and a portal admin will look at it.</p>
      </div>
      <p v-if="status === 'pending'" role="status" class="m-0 rounded-lg bg-panel px-3.5 py-3 text-sm font-medium">
        Requested {{ timeAgo(request!.created_at).toLowerCase() }}
      </p>
      <p v-else-if="status === 'approved'" class="m-0 text-sm text-muted">
        Approved on {{ fmtDate(request!.decided_at ?? '') }}, but you no longer have access.
      </p>
      <p v-else-if="status === 'declined'" class="m-0 text-sm text-muted">Declined on {{ fmtDate(request!.decided_at ?? '') }}.</p>
      <form v-if="canAsk" id="na-form" class="flex flex-col gap-1.5" @submit.prevent="ask">
        <label for="na-note" class="label">Note <span class="font-normal text-subtle">(optional)</span></label>
        <textarea
          id="na-note"
          v-model="note"
          rows="2"
          maxlength="500"
          placeholder="What you need it for"
          class="field resize-y"
        />
        <p v-if="error" role="alert" class="status-error mt-1">{{ error }}</p>
      </form>
      <div class="flex flex-wrap gap-2.5 pt-1">
        <button v-if="canAsk" type="submit" form="na-form" class="btn btn-primary" :disabled="sending">
          {{ sending ? 'Sending...' : 'Request access' }}
        </button>
        <RouterLink to="/" class="btn btn-secondary">Back to catalog</RouterLink>
      </div>
      <p v-if="info.admin_email" class="m-0 flex flex-wrap items-center gap-2 text-[13px] text-subtle">
        Portal admins:
        <span class="font-mono text-ink">{{ info.admin_email }}</span>
        <button type="button" class="inline-flex cursor-pointer items-center gap-1 rounded-md px-1.5 py-0.5 font-medium text-accent hover:bg-accent-soft" @click="copyEmail">
          <Icon :name="copied ? 'check' : 'copy'" :size="14" />
          {{ copied ? 'Copied' : 'Copy' }}
        </button>
      </p>
    </div>
  </main>
</template>
