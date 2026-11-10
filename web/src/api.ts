export type Visibility = 'everyone' | 'groups'

export interface Status {
  up: boolean
  http_status: number
  latency_ms: number
  error: string
  target: string
  checked_at: string
  since: string
}

export interface App {
  id: string
  name: string
  description: string
  owner: string
  contact: string
  category: string
  runs_on: string
  version: string
  start_here: boolean
  access: string
  health_url: string
  urls: Record<string, string>
  links: Record<string, string>
  visibility: Visibility
  groups: string[]
  icon_color: string
  // Tabler icon name, shown when no icon is uploaded
  icon: string
  has_icon: boolean
  icon_version: number
  favorite: boolean
  // opens by everyone in the last 30 days
  opens_30d: number
  can_edit: boolean
  created_by: string
  updated_by: string
  created_at: string
  updated_at: string
  status: Status | null
}

export interface AppList {
  apps: App[]
  total: number
  // the viewer's last opened app ids, newest first, empty when searching
  recent: string[]
}

export interface Option {
  key: string
  label: string
  count: number
}

export type OptionKind = 'category' | 'runs_on' | 'environment'
export type IconColorMode = 'auto' | 'accent'

export interface Portal {
  name: string
  admin_email: string
  admin_group: string
  icon_palette: string[]
  icon_color_mode: IconColorMode
  new_app_visibility: Visibility
  categories: Option[]
  runs_on: Option[]
  environments: Option[]
}

export interface PublicPortal {
  name: string
  admin_group?: string
  login_sso?: boolean
  login_local?: boolean
}

export type UserSource = 'sso' | 'local'


export interface Me {
  user: {
    subject: string
    username: string
    name: string
    email: string
    groups: string[]
    is_admin: boolean
    source: UserSource | ''
  }
  login_enabled: boolean
}

export interface UserRef {
  subject: string
  name: string
  email: string
}

export interface GroupApp {
  id: string
  name: string
  visibility: Visibility
}

export interface Group {
  name: string
  description: string
  members: UserRef[]
  apps: GroupApp[]
  created_at: string
}

export interface User {
  subject: string
  username: string
  name: string
  email: string
  groups: string[]
  is_admin: boolean
  // null until the first sign-in
  last_login: string | null
  source: UserSource
}

export interface LocalUserInput {
  username: string
  name: string
  email: string
  is_admin: boolean
  groups: string[]
  password: string
}

export interface UserInput {
  name: string
  email: string
  is_admin: boolean
}

export interface Change {
  id: number
  at: string
  actor: string
  summary: string
}

export type AttentionKind = 'user_no_group' | 'app_no_contact' | 'app_no_groups' | 'app_no_owner' | 'group_no_members'

export interface Attention {
  kind: AttentionKind
  title: string
  detail: string
  target: string
}

export interface Usage {
  people: number
  opens: number
  searches: number
  missed: number
}

export interface MissedSearch {
  query: string
  people: number
  last_at: string
  hidden: { id: string; name: string; visibility: Visibility } | null
}

export type AccessStatus = 'pending' | 'approved' | 'declined'

export interface AccessRequest {
  id: number
  subject: string
  person: string
  app_id: string
  app_name: string
  note: string
  status: AccessStatus
  group: string
  decided_by: string
  created_at: string
  decided_at: string | null
}

export interface Dashboard {
  usage: Usage
  missed: MissedSearch[]
  apps: number
  apps_everyone: number
  groups: number
  users: number
  admins: number
  default_icons: number
  attention: Attention[]
  changes: Change[]
}

export interface AppInput {
  id: string
  name: string
  description: string
  owner: string
  contact: string
  category: string
  runs_on: string
  version: string
  start_here: boolean
  access: string
  health_url: string
  urls: Record<string, string>
  links: Record<string, string>
  visibility: Visibility
  groups: string[]
  icon_color: string
  icon: string
}

// what the owner team can change without an admin
export interface DetailsInput {
  description: string
  contact: string
  version: string
  access: string
  health_url: string
  urls: Record<string, string>
  links: Record<string, string>
  icon_color: string
  icon: string
}

export interface Team {
  name: string
  description: string
  // display names
  members: string[]
  apps: App[]
}

export interface GroupInput {
  name: string
  description: string
  members: string[]
  apps: string[]
}

export interface SettingsInput {
  name: string
  admin_email: string
  icon_color_mode: IconColorMode
  new_app_visibility: Visibility
}

export interface NoAccessBody {
  error: string
  app: { id: string; name: string }
  groups: string[]
  admin_email: string
  // the viewer's latest request for the app
  request: AccessRequest | null
}

export interface AppFilter {
  q?: string
  category?: string
  runs_on?: string
  group?: string
}

export interface ImportProblem {
  line: number
  app: string
  field: string
  message: string
}

export interface TablerSet {
  version: string
  // name, tags, category, svg body
  icons: { n: string; t?: string; c?: string; b: string }[]
}

export const tablerURL = (name: string) => `/api/icons/tabler/${encodeURIComponent(name)}.svg`

export interface ImportPlan {
  apps: {
    id: string
    name: string
    line: number
    exists: boolean
    problems: number
    visibility: Visibility
    groups: string[]
    icon_color: string
    icon: string
    // why the owner is not set, empty when fine
    owner_note: string
  }[]
  create: {
    categories: { key: string; label: string }[]
    runs_on: { key: string; label: string }[]
    environments: { key: string; label: string }[]
    groups: string[]
  }
  problems: ImportProblem[]
}

export interface ImportResult {
  added: string[]
  updated: string[]
  skipped: string[]
}

export class ApiError extends Error {
  status: number
  fields: Record<string, string>
  body: unknown

  constructor(status: number, message: string, body: unknown) {
    super(message)
    this.status = status
    this.body = body
    const f = (body as { fields?: unknown } | null)?.fields
    this.fields = f && typeof f === 'object' ? (f as Record<string, string>) : {}
  }
}

let onUnauthorized: () => void = () => {}

// set by the router so a 401 anywhere sends the user to sign in
export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn
}

interface RequestOpts {
  signal?: AbortSignal
  // throw on 401 instead of redirecting, used by the router guard
  throw401?: boolean
}

async function request<T>(method: string, path: string, body?: unknown, opts: RequestOpts = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  let payload: BodyInit | undefined
  if (body instanceof FormData) payload = body
  else if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
    payload = JSON.stringify(body)
  }
  const res = await fetch(path, { method, headers, body: payload, signal: opts.signal })
  if (res.status === 401 && !opts.throw401) {
    onUnauthorized()
    return new Promise<T>(() => {})
  }
  if (!res.ok) {
    let data: unknown = null
    try {
      data = await res.json()
    } catch {
      // not json
    }
    const msg = (data as { error?: string } | null)?.error || `${res.status} ${res.statusText}`
    throw new ApiError(res.status, msg, data)
  }
  if (res.status === 204) return undefined as T
  const text = await res.text()
  return (text ? JSON.parse(text) : undefined) as T
}

const enc = encodeURIComponent

function qs(params: Record<string, string | undefined>) {
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) if (v) p.set(k, v)
  const s = p.toString()
  return s ? `?${s}` : ''
}

export const api = {
  portal: () => request<PublicPortal>('GET', '/api/portal'),
  me: (opts?: RequestOpts) => request<Me>('GET', '/api/me', undefined, opts),
  tablerIcons: () => request<TablerSet>('GET', '/api/icons/tabler'),
  // 401 here means a wrong password, not a lost session
  loginLocal: (username: string, password: string) =>
    request<void>('POST', '/auth/local', { username, password }, { throw401: true }),
  changePassword: (current: string, password: string) => request<void>('PUT', '/api/me/password', { current, password }),
  settings: (opts?: RequestOpts) => request<Portal>('GET', '/api/settings', undefined, opts),
  apps: (filter: AppFilter = {}, signal?: AbortSignal) =>
    request<AppList>('GET', `/api/apps${qs({ ...filter })}`, undefined, { signal }),
  app: (id: string, signal?: AbortSignal) => request<App>('GET', `/api/apps/${enc(id)}`, undefined, { signal }),
  facets: (signal?: AbortSignal) =>
    request<{ categories: Option[]; runs_on: Option[] }>('GET', '/api/facets', undefined, { signal }),
  favorite: (id: string, on: boolean) => request<void>(on ? 'PUT' : 'DELETE', `/api/favorites/${enc(id)}`),
  // a settled search, errors don't matter
  logSearch: (q: string, found: boolean) =>
    request<void>('POST', '/api/searches', { q, found }, { throw401: true }).then(
      () => {},
      () => {},
    ),
  // for admins and the owner team
  updateDetails: (id: string, input: DetailsInput) => request<App>('PUT', `/api/apps/${enc(id)}/details`, input),
  uploadIcon: (id: string, file: File) => {
    const fd = new FormData()
    fd.append('icon', file)
    return request<App>('PUT', `/api/apps/${enc(id)}/icon`, fd)
  },
  deleteIcon: (id: string) => request<App>('DELETE', `/api/apps/${enc(id)}/icon`),
  team: (name: string, signal?: AbortSignal) => request<Team>('GET', `/api/teams/${enc(name)}`, undefined, { signal }),
  requestAccess: (id: string, note: string) =>
    request<AccessRequest>('POST', `/api/apps/${enc(id)}/access-requests`, { note }),

  dashboard: () => request<Dashboard>('GET', '/api/admin/dashboard'),
  dismissSearch: (q: string) => request<void>('POST', '/api/admin/searches/dismiss', { q }),
  accessRequests: () => request<{ requests: AccessRequest[] }>('GET', '/api/admin/access-requests'),
  approveAccess: (id: number, group: string) =>
    request<AccessRequest>('POST', `/api/admin/access-requests/${id}/approve`, { group }),
  declineAccess: (id: number) => request<AccessRequest>('POST', `/api/admin/access-requests/${id}/decline`),
  createApp: (input: AppInput) => request<App>('POST', '/api/admin/apps', input),
  updateApp: (id: string, input: AppInput) => request<App>('PUT', `/api/admin/apps/${enc(id)}`, input),
  deleteApp: (id: string) => request<void>('DELETE', `/api/admin/apps/${enc(id)}`),
  groups: () => request<{ groups: Group[] }>('GET', '/api/admin/groups'),
  group: (name: string) => request<Group>('GET', `/api/admin/groups/${enc(name)}`),
  createGroup: (input: GroupInput) => request<Group>('POST', '/api/admin/groups', input),
  updateGroup: (name: string, input: GroupInput) => request<Group>('PUT', `/api/admin/groups/${enc(name)}`, input),
  deleteGroup: (name: string) => request<void>('DELETE', `/api/admin/groups/${enc(name)}`),
  users: () => request<{ users: User[] }>('GET', '/api/admin/users'),
  setUserGroups: (subject: string, groups: string[]) =>
    request<User>('PUT', `/api/admin/users/${enc(subject)}/groups`, { groups }),
  createUser: (input: LocalUserInput) => request<User>('POST', '/api/admin/users', input),
  updateUser: (subject: string, input: UserInput) => request<User>('PUT', `/api/admin/users/${enc(subject)}`, input),
  setUserPassword: (subject: string, password: string) =>
    request<void>('PUT', `/api/admin/users/${enc(subject)}/password`, { password }),
  deleteUser: (subject: string) => request<void>('DELETE', `/api/admin/users/${enc(subject)}`),
  saveSettings: (input: SettingsInput) => request<Portal>('PUT', '/api/admin/settings', input),
  addOption: (kind: OptionKind, label: string) => request<Portal>('POST', `/api/admin/options/${kind}`, { label }),
  removeOption: (kind: OptionKind, key: string) => request<Portal>('DELETE', `/api/admin/options/${kind}/${enc(key)}`),
  renameOption: (kind: OptionKind, key: string, label: string) =>
    request<Portal>('PUT', `/api/admin/options/${kind}/${enc(key)}`, { label }),
  reorderOptions: (kind: OptionKind, keys: string[]) => request<Portal>('PUT', `/api/admin/options/${kind}`, { keys }),
  // one check right now, nothing is saved
  healthCheck: (input: { health_url: string; urls: Record<string, string> }) =>
    request<Status>('POST', '/api/admin/health-check', input),
  // checks an import file, nothing is saved
  previewImport: (yaml: string) => request<ImportPlan>('POST', '/api/admin/import/preview', { yaml }),
  runImport: (yaml: string, existing: 'skip' | 'update') =>
    request<ImportResult>('POST', '/api/admin/import', { yaml, existing }),
}

// counts an open, fire and forget so it still goes out when the page navigates
export function trackOpen(id: string) {
  fetch(`/api/apps/${enc(id)}/open`, { method: 'POST', keepalive: true, credentials: 'same-origin' }).catch(() => {})
}

// message for an error shown in the page, '' when it should stay quiet
export function errorText(e: unknown, fallback = 'Something went wrong. Try again.') {
  if ((e as Error)?.name === 'AbortError') return ''
  if (e instanceof ApiError) return e.message || fallback
  return fallback
}

export const isAbort = (e: unknown) => (e as Error)?.name === 'AbortError'

export const LINK_KINDS = [
  { key: 'repo', label: 'Repository' },
  { key: 'docs', label: 'Docs' },
  { key: 'ci', label: 'CI pipeline' },
] as const

// environments in settings order, unknown keys last
export function orderedURLs(app: App, envs: Option[] = []): [string, string][] {
  const rank = (k: string) => {
    const i = envs.findIndex((e) => e.key === k)
    return i === -1 ? 999 : i
  }
  return Object.entries(app.urls)
    .filter(([, v]) => v)
    .sort(([a], [b]) => rank(a) - rank(b) || a.localeCompare(b))
}

// prod, else the first one in environment order
export function primaryURL(app: App, envs: Option[] = []): string | undefined {
  if (app.urls.prod) return app.urls.prod
  return orderedURLs(app, envs)[0]?.[1]
}

export function displayURL(url: string) {
  return url.replace(/^https?:\/\//, '').replace(/\/$/, '')
}

export function hostOf(url: string) {
  try {
    return new URL(/^[a-z]+:\/\//i.test(url) ? url : `https://${url}`).host
  } catch {
    return url
  }
}

export function href(url: string) {
  return /^[a-z][a-z0-9+.-]*:/i.test(url) ? url : `https://${url}`
}

export const isEmail = (s: string) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(s)

export function initials(name: string) {
  const words = name
    .replace(/\(.*?\)/g, '')
    .trim()
    .split(/\s+/)
    .filter(Boolean)
  if (words.length === 0) return '?'
  const s = words.length > 1 ? words[0]![0]! + words[1]![0]! : words[0]!.slice(0, 2)
  return s.toUpperCase()
}

export function iconColor(app: { id: string; icon_color: string }, palette: string[], mode: IconColorMode) {
  if (palette.length === 0) return '#2251C7'
  if (mode === 'accent') return palette[0]!
  if (app.icon_color) return app.icon_color
  let h = 0
  for (const ch of app.id) h += ch.charCodeAt(0)
  return palette[h % palette.length]!
}

export function slugify(s: string) {
  return s
    .toLowerCase()
    .replace(/\(.*?\)/g, ' ')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

export const ID_PATTERN = /^[a-z0-9]+(-[a-z0-9]+)*$/

export const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`

const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

export function timeAgo(iso: string, now = Date.now()) {
  const t = new Date(iso).getTime()
  if (!iso || Number.isNaN(t)) return ''
  const sec = Math.round((t - now) / 1000)
  const abs = Math.abs(sec)
  let out: string
  if (abs < 60) out = 'just now'
  else if (abs < 3600) out = rtf.format(Math.round(sec / 60), 'minute')
  else if (abs < 86400) out = rtf.format(Math.round(sec / 3600), 'hour')
  else if (abs < 7 * 86400) out = rtf.format(Math.round(sec / 86400), 'day')
  else if (abs < 30 * 86400) out = rtf.format(Math.round(sec / (7 * 86400)), 'week')
  else if (abs < 365 * 86400) out = rtf.format(Math.round(sec / (30 * 86400)), 'month')
  else out = rtf.format(Math.round(sec / (365 * 86400)), 'year')
  return cap(out)
}

// calendar days: Today, Yesterday, 3 days ago, 2 weeks ago
export function daysAgo(iso: string | null, now = new Date()) {
  if (!iso) return 'Never'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return 'Never'
  const startOf = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime()
  const days = Math.round((startOf(now) - startOf(d)) / 86400000)
  if (days <= 0) return 'Today'
  if (days < 7) return cap(rtf.format(-days, 'day'))
  if (days < 30) return cap(rtf.format(-Math.round(days / 7), 'week'))
  if (days < 365) return cap(rtf.format(-Math.round(days / 30), 'month'))
  return cap(rtf.format(-Math.round(days / 365), 'year'))
}

export function fmtDate(iso: string) {
  const d = new Date(iso)
  if (!iso || Number.isNaN(d.getTime())) return ''
  return d.toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })
}

// "Down since 12 minutes ago, checked just now" style details for a tooltip
export function statusTitle(s: Status | null) {
  if (!s) return 'No health check result yet.'
  const detail = s.up ? `HTTP ${s.http_status}, ${s.latency_ms} ms` : s.error || `HTTP ${s.http_status}`
  return `${detail}. ${s.up ? 'Up' : 'Down'} since ${timeAgo(s.since).toLowerCase()}, checked ${timeAgo(s.checked_at).toLowerCase()}.`
}

export async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text)
  } catch {
    // clipboard api needs https, fall back for plain http
    const ta = document.createElement('textarea')
    ta.value = text
    ta.style.position = 'fixed'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    ta.remove()
  }
}

// "a,b" from the url query into ["a", "b"]
export function splitKeys(v: unknown): string[] {
  return String(v ?? '')
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
}

function editDistance(a: string, b: string) {
  const row = Array.from({ length: b.length + 1 }, (_, i) => i)
  for (let i = 1; i <= a.length; i++) {
    let prev = row[0]!
    row[0] = i
    for (let j = 1; j <= b.length; j++) {
      const tmp = row[j]!
      row[j] = Math.min(row[j]! + 1, row[j - 1]! + 1, prev + (a[i - 1] === b[j - 1] ? 0 : 1))
      prev = tmp
    }
  }
  return row[b.length]!
}

const words = (s: string) => s.toLowerCase().split(/[^a-z0-9]+/).filter(Boolean)

// apps whose name or id is close to the text, best first, for typos and renamed apps
export function similarApps<T extends { id: string; name: string }>(apps: T[], text: string, max = 3): T[] {
  const want = words(text)
  if (!want.length) return []
  const scored = apps.map((a) => {
    const have = [...words(a.name), ...words(a.id)]
    let score = 0
    let top = 0
    for (const w of want) {
      let best = 0
      for (const h of have) {
        if (h === w) best = Math.max(best, 1)
        else if (h.startsWith(w) || w.startsWith(h)) best = Math.max(best, 0.8)
        else {
          const d = editDistance(w, h)
          const sim = 1 - d / Math.max(w.length, h.length)
          if (sim >= 0.6) best = Math.max(best, sim * 0.8)
        }
      }
      score += best
      top = Math.max(top, best)
    }
    // one strong word counts most, extra words like "old" shouldn't sink it
    return { a, score: top * 0.7 + (score / want.length) * 0.3 }
  })
  return scored
    .filter((s) => s.score >= 0.45)
    .sort((x, y) => y.score - x.score)
    .slice(0, max)
    .map((s) => s.a)
}
