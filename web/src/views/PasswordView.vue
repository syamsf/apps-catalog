<script setup lang="ts">
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'
import Avatar from '@/components/Avatar.vue'
import Icon from '@/components/Icon.vue'
import PasswordInput from '@/components/PasswordInput.vue'
import SiteHeader from '@/components/SiteHeader.vue'
import { ApiError, api, errorText } from '@/api'
import { isLocalUser, me, userName } from '@/session'

const current = ref('')
const next = ref('')
const again = ref('')
const errors = ref<Record<string, string>>({})
const error = ref('')
const done = ref(false)
const busy = ref(false)
const longEnough = computed(() => next.value.length >= 8)
const matches = computed(() => !!again.value && again.value === next.value)

async function save() {
  error.value = ''
  done.value = false
  const e: Record<string, string> = {}
  if (!current.value) e.current = 'Enter your current password'
  if (next.value.length < 8) e.password = 'Use at least 8 characters'
  else if (next.value !== again.value) e.again = "The new passwords don't match"
  errors.value = e
  if (Object.keys(e).length) return
  busy.value = true
  try {
    await api.changePassword(current.value, next.value)
    current.value = next.value = again.value = ''
    done.value = true
  } catch (err) {
    if (err instanceof ApiError && Object.keys(err.fields).length) errors.value = err.fields
    else error.value = errorText(err, 'Could not change the password. Try again.')
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="min-h-dvh">
    <SiteHeader />
    <main class="mx-auto flex max-w-[480px] flex-col gap-5 px-4 py-10">
      <div class="flex flex-col gap-1">
        <RouterLink to="/" class="inline-flex min-h-8 items-center gap-1.5 self-start text-sm font-medium text-accent no-underline hover:text-accent-ink">
          <Icon name="back" :size="16" />
          All apps
        </RouterLink>
        <h1 class="m-0 text-[28px] font-semibold">Change password</h1>
      </div>
      <p v-if="!isLocalUser" class="status-error">You sign in with company SSO. Change your password there.</p>
      <form v-else class="card flex flex-col gap-[18px] p-6" novalidate @submit.prevent="save">
        <div class="flex items-center gap-2.5 rounded-[10px] border border-line-soft bg-panel px-3 py-2.5 text-[13px] text-subtle">
          <Avatar :name="userName" :size="28" />
          Portal account:
          <span class="font-mono font-medium text-ink">{{ me?.user.username }}</span>
        </div>
        <p v-if="done" role="status" class="status-ok">Your password is changed.</p>
        <p v-if="error" role="alert" class="status-error">{{ error }}</p>
        <div class="flex flex-col gap-1.5">
          <label for="p-cur" class="label">Current password</label>
          <PasswordInput id="p-cur" v-model="current" autocomplete="current-password" :invalid="!!errors.current" />
          <span v-if="errors.current" class="field-error">{{ errors.current }}</span>
        </div>
        <div class="flex flex-col gap-1.5">
          <label for="p-new" class="label">New password</label>
          <PasswordInput id="p-new" v-model="next" autocomplete="new-password" :invalid="!!errors.password" />
          <span v-if="errors.password" class="field-error">{{ errors.password }}</span>
          <span v-else class="inline-flex items-center gap-1.5 text-[12.5px]" :class="longEnough ? 'font-medium text-ok' : 'text-subtle'">
            <Icon name="check" :size="14" :class="longEnough ? '' : 'text-faint'" />
            At least 8 characters
          </span>
        </div>
        <div class="flex flex-col gap-1.5">
          <label for="p-again" class="label">New password again</label>
          <PasswordInput id="p-again" v-model="again" autocomplete="new-password" :invalid="!!errors.again" />
          <span v-if="errors.again" class="field-error">{{ errors.again }}</span>
          <span v-else-if="again" class="inline-flex items-center gap-1.5 text-[12.5px]" :class="matches ? 'font-medium text-ok' : 'text-subtle'">
            <Icon name="check" :size="14" :class="matches ? '' : 'text-faint'" />
            {{ matches ? 'Passwords match' : "Passwords don't match yet" }}
          </span>
        </div>
        <div class="flex gap-2.5">
          <button type="submit" class="btn btn-primary" :disabled="busy">
            {{ busy ? 'Saving...' : 'Change password' }}
          </button>
          <RouterLink to="/" class="btn btn-ghost">Cancel</RouterLink>
        </div>
      </form>
    </main>
  </div>
</template>
