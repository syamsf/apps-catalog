<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Icon from '@/components/Icon.vue'
import OptionList from '@/components/OptionList.vue'
import SaveBar from '@/components/SaveBar.vue'
import { type App, type IconColorMode, type Portal, type Visibility, api, errorText, initials } from '@/api'
import { portalInfo, setSettings, settings } from '@/session'

const route = useRoute()
const router = useRouter()

const tabs = [
  { key: 'general', label: 'General' },
  { key: 'catalog', label: 'Catalog options' },
  { key: 'defaults', label: 'Defaults' },
] as const
type Tab = (typeof tabs)[number]['key']
const tab = computed<Tab>(() => (tabs.some((t) => t.key === route.query.tab) ? (route.query.tab as Tab) : 'general'))
const setTab = (t: Tab) => router.replace({ query: { ...route.query, tab: t === 'general' ? undefined : t } })
function onTabKey(e: KeyboardEvent) {
  const i = tabs.findIndex((t) => t.key === tab.value)
  const j = e.key === 'ArrowRight' ? i + 1 : e.key === 'ArrowLeft' ? i - 1 : -9
  if (j === -9) return
  const next = tabs[(j + tabs.length) % tabs.length]!
  setTab(next.key)
  document.getElementById(`tab-${next.key}`)?.focus()
}

const fromSettings = () => ({
  name: settings.value?.name ?? '',
  admin_email: settings.value?.admin_email ?? '',
  icon_color_mode: (settings.value?.icon_color_mode ?? 'auto') as IconColorMode,
  new_app_visibility: (settings.value?.new_app_visibility ?? 'groups') as Visibility,
})
const form = reactive(fromSettings())
const saved = ref(fromSettings())
const saving = ref(false)
const error = ref('')

const labels = { name: 'Portal name', admin_email: 'Admin contact email', icon_color_mode: 'Default icon', new_app_visibility: 'New apps' }
const changed = computed(() =>
  (Object.keys(labels) as (keyof typeof labels)[]).filter((k) => String(form[k]).trim() !== String(saved.value[k]).trim()).map((k) => labels[k]),
)

async function save() {
  if (saving.value) return
  saving.value = true
  error.value = ''
  try {
    const s = await api.saveSettings({ ...form, name: form.name.trim(), admin_email: form.admin_email.trim() })
    setSettings(s)
    saved.value = fromSettings()
    say('Settings saved.')
  } catch (e) {
    error.value = errorText(e, 'Could not save the settings. Try again.')
  } finally {
    saving.value = false
  }
}
function cancel() {
  Object.assign(form, saved.value)
  error.value = ''
}

// the lists save right away, this tells the admin it happened
const notice = ref('')
let timer: ReturnType<typeof setTimeout> | undefined
function say(text: string) {
  notice.value = text
  clearTimeout(timer)
  timer = setTimeout(() => (notice.value = ''), 4000)
}
onBeforeUnmount(() => clearTimeout(timer))
function onListSaved(s: Portal, what: string) {
  setSettings(s)
  say(`${what}. Saved.`)
}

// real apps for the icon preview
const apps = ref<App[]>([])
api
  .apps()
  .then((r) => (apps.value = r.apps))
  .catch(() => {})
const palette = computed(() => settings.value?.icon_palette ?? [])
const samples = computed(() =>
  palette.value.map((c, i) => {
    const a = apps.value[i]
    return { color: c, app: { id: a?.id ?? `sample-${i}`, name: a?.name ?? 'App', icon_color: form.icon_color_mode === 'accent' ? palette.value[0]! : c } }
  }),
)
</script>

<template>
  <h1 class="m-0 text-[28px] font-semibold">Settings</h1>

  <div role="tablist" aria-label="Settings" class="-mt-1 flex gap-1 border-b border-line" @keydown="onTabKey">
    <button
      v-for="t in tabs"
      :id="`tab-${t.key}`"
      :key="t.key"
      type="button"
      role="tab"
      :aria-selected="tab === t.key"
      :aria-controls="`panel-${t.key}`"
      :tabindex="tab === t.key ? 0 : -1"
      class="-mb-px cursor-pointer border-b-2 px-3 py-2.5 text-sm"
      :class="tab === t.key ? 'border-accent font-semibold text-accent-ink' : 'border-transparent font-medium text-subtle hover:text-ink'"
      @click="setTab(t.key)"
    >
      {{ t.label }}
    </button>
  </div>

  <p v-if="!settings" role="alert" class="status-error">Could not load the settings. Reload the page to try again.</p>

  <template v-else>
    <p v-if="notice" role="status" class="m-0 inline-flex items-center gap-1.5 self-start text-[13px] font-medium text-ok">
      <Icon name="check" :size="14" />
      {{ notice }}
    </p>
    <p v-if="error" role="alert" class="status-error">{{ error }}</p>

    <form v-if="tab === 'general'" id="settings-form" role="tabpanel" aria-labelledby="tab-general" class="flex max-w-[760px] flex-col gap-5" @submit.prevent="save">
      <section aria-labelledby="s-portal" class="card flex flex-col gap-4 p-6">
        <h2 id="s-portal" class="section-title text-[17px]">Portal</h2>
        <div class="grid gap-4 sm:grid-cols-2">
          <div class="flex flex-col gap-1.5">
            <label for="s-name" class="label">Portal name</label>
            <input id="s-name" v-model="form.name" type="text" class="field" />
            <span class="hint">Shown in the header and the browser tab.</span>
          </div>
          <div class="flex flex-col gap-1.5">
            <label for="s-email" class="label">Admin contact email</label>
            <input id="s-email" v-model="form.admin_email" type="email" class="field" />
            <span class="hint">Shown when someone opens an app they can't access.</span>
          </div>
        </div>
      </section>
      <section aria-labelledby="s-signin" class="card flex flex-col gap-1 px-6 pt-5 pb-4">
        <h2 id="s-signin" class="section-title text-[17px]">Sign-in</h2>
        <p class="m-0 mb-2 text-[13px] text-subtle">Set in the config file. Shown here so you know what users see.</p>
        <div class="flex items-center gap-4 border-b border-line-soft py-3">
          <span class="flex flex-1 flex-col gap-0.5">
            <span class="text-sm font-medium">Company SSO</span>
            <span class="text-[12.5px] text-subtle">Sign in through the company login page</span>
          </span>
          <span :class="portalInfo?.login_sso ? 'chip-ok' : 'chip'">{{ portalInfo?.login_sso ? 'On' : 'Off' }}</span>
        </div>
        <div class="flex items-center gap-4 border-b border-line-soft py-3">
          <span class="flex flex-1 flex-col gap-0.5">
            <span class="text-sm font-medium">Portal accounts</span>
            <span class="text-[12.5px] text-subtle">Username and password, managed on the Users page</span>
          </span>
          <span :class="portalInfo?.login_local ? 'chip-ok' : 'chip'">{{ portalInfo?.login_local ? 'On' : 'Off' }}</span>
        </div>
        <div class="flex items-center gap-4 py-3">
          <span class="flex flex-1 flex-col gap-0.5">
            <span class="text-sm font-medium">Admin group in LLDAP</span>
            <span class="text-[12.5px] text-subtle">Members of this group are portal admins</span>
          </span>
          <span v-if="settings.admin_group" class="chip-group">{{ settings.admin_group }}</span>
          <span v-else class="text-[13px] text-faint">Not set</span>
        </div>
        <p class="m-0 flex items-center gap-1.5 text-[12.5px] text-faint">
          <Icon name="lock" :size="14" />
          Read only. Change these in the config file and restart the portal.
        </p>
      </section>
    </form>

    <div v-else-if="tab === 'catalog'" id="panel-catalog" role="tabpanel" aria-labelledby="tab-catalog" class="flex flex-col gap-5 xl:flex-row xl:items-start">
      <div class="min-w-0 flex-1">
        <OptionList
          kind="environment"
          title="Environments"
          help="One URL field per environment on the app form. Drag to set the order shown on app pages. Open app uses Production, or the first environment an app has a URL for."
          add-label="Add environment, e.g. Pre-prod"
          :items="settings.environments"
          ordered
          tag-key="prod"
          tag="Open app default"
          @saved="onListSaved"
        />
      </div>
      <div class="flex flex-col gap-5 xl:w-[400px] xl:shrink-0">
        <OptionList
          kind="category"
          title="Categories"
          help="Used for the category filter in the catalog."
          add-label="Add category"
          :items="settings.categories"
          @saved="onListSaved"
        />
        <OptionList
          kind="runs_on"
          title="Runs on"
          help="Where apps are hosted. Shown on each app and used as a filter."
          add-label="Add option"
          :items="settings.runs_on"
          @saved="onListSaved"
        />
      </div>
    </div>

    <form v-else id="settings-form" role="tabpanel" aria-labelledby="tab-defaults" class="flex max-w-[760px] flex-col gap-5" @submit.prevent="save">
      <section aria-labelledby="s-icon" class="card flex flex-col gap-3.5 p-6">
        <div class="flex flex-col gap-1">
          <h2 id="s-icon" class="section-title text-[17px]">Default app icon</h2>
          <p class="m-0 text-[13px] text-subtle">Apps without an uploaded icon show their initials on a color.</p>
        </div>
        <div class="flex flex-col">
          <label class="flex min-h-11 cursor-pointer items-center gap-2.5 text-sm">
            <input v-model="form.icon_color_mode" type="radio" name="icon-color" value="auto" class="check" />
            A color per app, picked from the app ID
          </label>
          <label class="flex min-h-11 cursor-pointer items-center gap-2.5 text-sm">
            <input v-model="form.icon_color_mode" type="radio" name="icon-color" value="accent" class="check" />
            The accent color for every app
          </label>
        </div>
        <div class="flex flex-col gap-2.5 rounded-[10px] border border-line-soft bg-panel p-4">
          <span class="text-xs font-semibold text-subtle">The colors it picks from</span>
          <div class="flex flex-wrap gap-3.5">
            <div v-for="s in samples" :key="s.color" class="flex w-[76px] flex-col items-center gap-1.5">
              <span
                aria-hidden="true"
                class="flex size-11 items-center justify-center rounded-[10px] text-[15px] font-semibold text-white"
                :style="{ background: s.app.icon_color }"
                >{{ initials(s.app.name) }}</span
              >
              <span class="font-mono text-[11px] text-subtle">{{ s.color.toUpperCase() }}</span>
            </div>
          </div>
        </div>
      </section>
      <section aria-labelledby="s-new" class="card flex flex-col gap-2.5 p-6">
        <div class="flex flex-col gap-1">
          <h2 id="s-new" class="section-title text-[17px]">New apps</h2>
          <p class="m-0 text-[13px] text-subtle">What "Who can see this app" starts as on the Add app form.</p>
        </div>
        <div class="flex flex-col">
          <label class="flex min-h-11 cursor-pointer items-center gap-2.5 text-sm">
            <input v-model="form.new_app_visibility" type="radio" name="new-vis" value="groups" class="check" />
            Only selected groups (safer)
          </label>
          <label class="flex min-h-11 cursor-pointer items-center gap-2.5 text-sm">
            <input v-model="form.new_app_visibility" type="radio" name="new-vis" value="everyone" class="check" />
            Everyone who signs in
          </label>
        </div>
      </section>
    </form>
  </template>

  <SaveBar
    :show="tab !== 'catalog' && changed.length > 0"
    label="Unsaved changes"
    :detail="changed.join(', ')"
    :saving="saving"
    action="Save settings"
    form="settings-form"
    @cancel="cancel"
  />
</template>
