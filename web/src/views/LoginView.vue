<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import Icon from '@/components/Icon.vue'
import PasswordInput from '@/components/PasswordInput.vue'
import { ApiError, type PublicPortal, api, errorText } from '@/api'
import { loadPortalName, portalName, settings } from '@/session'

const props = defineProps<{ admin?: boolean }>()
const route = useRoute()

const info = ref<PublicPortal | null>(null)
const loaded = ref(false)
const adminGroup = computed(() => info.value?.admin_group || settings.value?.admin_group || '')
const sso = computed(() => !!info.value?.login_sso)
const local = computed(() => !!info.value?.login_local)

onMounted(async () => {
  info.value = await loadPortalName()
  loaded.value = true
})

// only same-site paths
const redirect = computed(() => {
  const r = String(route.query.redirect ?? '')
  if (r.startsWith('/') && !r.startsWith('//')) return r
  return props.admin ? '/admin' : '/'
})
const loginHref = computed(() => `/auth/login?redirect=${encodeURIComponent(redirect.value)}`)
const signedOut = computed(() => 'signed_out' in route.query)

// with sso on, the portal account form starts folded
const showLocal = ref('local' in route.query)
const formOpen = computed(() => local.value && (!sso.value || showLocal.value))
const userInput = ref<HTMLInputElement>()
async function openLocal() {
  showLocal.value = true
  await nextTick()
  userInput.value?.focus()
}

const username = ref('')
const password = ref('')
const busy = ref(false)
const error = ref('')

async function signIn() {
  error.value = ''
  if (!username.value.trim() || !password.value) {
    error.value = 'Enter your username and password.'
    return
  }
  busy.value = true
  try {
    await api.loginLocal(username.value.trim(), password.value)
    // full load so the new session is picked up everywhere
    window.location.assign(redirect.value)
  } catch (e) {
    password.value = ''
    error.value =
      e instanceof ApiError && e.status === 401 ? 'Wrong username or password.' : errorText(e, 'Could not sign in. Try again.')
    busy.value = false
  }
}
</script>

<template>
  <div class="flex min-h-dvh items-center justify-center px-4 py-12">
    <div class="flex w-full max-w-[440px] flex-col gap-5">
      <div class="flex flex-col gap-6 rounded-2xl border border-line bg-white px-6 py-10 sm:px-9">
        <div class="flex items-center gap-2.5">
          <span class="flex size-9 items-center justify-center rounded-[9px] bg-accent text-white">
            <Icon name="grid" :size="20" />
          </span>
          <span class="text-lg font-semibold">{{ portalName }}</span>
          <span v-if="admin" class="rounded-md bg-ink px-2.5 py-1 text-xs font-semibold tracking-[0.04em] text-white">ADMIN</span>
        </div>
        <p v-if="signedOut" role="status" class="status-ok">You're signed out.</p>
        <div class="flex flex-col gap-2">
          <h1 class="m-0 text-[26px] leading-tight font-semibold">
            {{ admin ? 'Sign in to the admin area' : 'Find any internal app' }}
          </h1>
          <p class="m-0 text-[15px] leading-normal text-muted">
            <template v-if="admin">Add and edit apps, manage groups, and choose which groups see which app.</template>
            <template v-else>
              Every internal app in one place. Search, open it in one click, and stop hunting for domains and IPs.
            </template>
          </p>
        </div>

        <div v-if="sso" class="flex flex-col gap-2.5">
          <a :href="loginHref" class="btn min-h-12 text-[15px]" :class="formOpen ? 'btn-secondary' : 'btn-primary'">
            <Icon name="lock" />
            Sign in with company SSO
          </a>
          <p class="m-0 text-[13px] leading-normal text-subtle">You'll sign in on the company login page, then come back here.</p>
        </div>

        <button
          v-if="sso && local && !formOpen"
          type="button"
          class="-mt-2 inline-flex min-h-10 cursor-pointer items-center justify-center gap-1.5 text-sm font-medium text-accent hover:text-accent-ink"
          :aria-expanded="false"
          aria-controls="local-form"
          @click="openLocal"
        >
          Use a portal account instead
          <Icon name="chevron" :size="16" />
        </button>

        <div v-if="sso && formOpen" class="flex items-center gap-3 text-[13px] font-medium text-subtle">
          <span class="h-px flex-1 bg-line"></span>
          Portal account
          <span class="h-px flex-1 bg-line"></span>
        </div>

        <form v-if="formOpen" id="local-form" class="flex flex-col gap-4" novalidate @submit.prevent="signIn">
          <p v-if="error" role="alert" class="status-error">{{ error }}</p>
          <div class="flex flex-col gap-1.5">
            <label for="l-user" class="label">Username</label>
            <input
              id="l-user"
              ref="userInput"
              v-model="username"
              type="text"
              name="username"
              autocomplete="username"
              autocapitalize="none"
              spellcheck="false"
              class="field"
            />
          </div>
          <div class="flex flex-col gap-1.5">
            <label for="l-pass" class="label">Password</label>
            <PasswordInput id="l-pass" v-model="password" name="password" autocomplete="current-password" />
          </div>
          <button type="submit" class="btn btn-primary min-h-12 text-[15px]" :disabled="busy">
            {{ busy ? 'Signing in...' : 'Sign in' }}
          </button>
        </form>

        <p v-if="loaded && !sso && !local" class="m-0 text-sm text-muted">
          Sign-in is off on this portal. <a :href="redirect" class="link">Continue</a>
        </p>

        <p v-if="admin" class="m-0 rounded-lg bg-chip px-3.5 py-3 text-[13px] leading-normal text-muted">
          <template v-if="sso && adminGroup">
            Only members of the LLDAP group <span class="font-mono text-ink">{{ adminGroup }}</span>
            <template v-if="local"> and portal accounts with the admin role</template> can open the admin area.
          </template>
          <template v-else>Only portal admins can open the admin area.</template>
          Anyone else is sent to the catalog.
        </p>
      </div>
      <p class="m-0 text-center text-[13px] text-faint">
        <template v-if="admin">
          Not an admin?
          <RouterLink to="/login" class="font-medium text-subtle no-underline hover:text-ink hover:underline">Catalog sign-in</RouterLink>
        </template>
        <template v-else>
          Portal admin?
          <RouterLink to="/admin/login" class="font-medium text-subtle no-underline hover:text-ink hover:underline">Admin sign-in</RouterLink>
        </template>
      </p>
    </div>
  </div>
</template>
