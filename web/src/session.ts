import { computed, ref, watchEffect } from 'vue'
import { type Me, type Option, type Portal, type PublicPortal, api } from '@/api'

export const me = ref<Me | null>(null)
export const settings = ref<Portal | null>(null)
// public info: sign-in methods and admin group
export const portalInfo = ref<PublicPortal | null>(null)
export const portalName = ref('')
export const pageTitle = ref('')

// one-shot message shown on the next page, e.g. after a save
export const flash = ref<{ text: string; error?: boolean } | null>(null)

export const isAdmin = computed(() => !!me.value?.user.is_admin)
export const userName = computed(() => me.value?.user.name || me.value?.user.username || '')
export const isLocalUser = computed(() => me.value?.user.source === 'local')
export const localLoginOn = computed(() => !!portalInfo.value?.login_local)

let sessionLoad: Promise<void> | null = null

// loads the signed-in user and settings once, throws ApiError(401) when signed out
export function ensureSession() {
  if (!sessionLoad) {
    sessionLoad = Promise.all([api.me({ throw401: true }), api.settings({ throw401: true }), loadPortalName()])
      .then(([m, s]) => {
        me.value = m
        setSettings(s)
      })
      .catch((e) => {
        sessionLoad = null
        throw e
      })
  }
  return sessionLoad
}

export function setSettings(s: Portal) {
  settings.value = s
  if (s.name) portalName.value = s.name
}

export async function loadPortalName() {
  try {
    const p = await api.portal()
    portalInfo.value = p
    if (!portalName.value) portalName.value = p.name
    return p
  } catch {
    return null
  }
}

export function optionLabel(list: Option[] | undefined, key: string) {
  if (!key) return ''
  return list?.find((o) => o.key === key)?.label ?? key
}

watchEffect(() => {
  const name = portalName.value
  const page = pageTitle.value
  document.title = page && name ? `${page} - ${name}` : page || name || 'Portal'
})

export function setTitle(page: string) {
  pageTitle.value = page
}
