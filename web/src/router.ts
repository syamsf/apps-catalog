import { createRouter, createWebHistory } from 'vue-router'
import { ApiError, setUnauthorizedHandler } from '@/api'
import { ensureSession, isAdmin, setTitle } from '@/session'
import CatalogView from '@/views/CatalogView.vue'

declare module 'vue-router' {
  interface RouteMeta {
    public?: boolean
    admin?: boolean
    title?: string
  }
}

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: () => import('@/views/LoginView.vue'), meta: { public: true, title: 'Sign in' } },
    {
      path: '/admin/login',
      name: 'admin-login',
      component: () => import('@/views/LoginView.vue'),
      props: { admin: true },
      meta: { public: true, title: 'Admin sign in' },
    },
    { path: '/', name: 'catalog', component: CatalogView, meta: { title: 'App catalog' } },
    { path: '/apps/:id', name: 'app', component: () => import('@/views/AppDetailView.vue'), props: true },
    {
      path: '/apps/:id/edit',
      name: 'app-edit',
      component: () => import('@/views/admin/AppFormView.vue'),
      props: (to) => ({ id: String(to.params.id), ownerMode: true }),
      // admins get the full form
      beforeEnter: (to) => (isAdmin.value ? `/admin/apps/${encodeURIComponent(String(to.params.id))}/edit` : true),
      meta: { title: 'Edit app' },
    },
    { path: '/teams/:name', name: 'team', component: () => import('@/views/TeamView.vue'), props: true, meta: { title: 'Team' } },
    {
      path: '/account/password',
      name: 'password',
      component: () => import('@/views/PasswordView.vue'),
      meta: { title: 'Change password' },
    },
    {
      path: '/admin',
      component: () => import('@/views/admin/AdminLayout.vue'),
      meta: { admin: true },
      children: [
        { path: '', name: 'admin', component: () => import('@/views/admin/DashboardView.vue'), meta: { title: 'Dashboard' } },
        { path: 'apps', name: 'admin-apps', component: () => import('@/views/admin/AppsView.vue'), meta: { title: 'Apps' } },
        {
          path: 'apps/new',
          name: 'admin-app-new',
          component: () => import('@/views/admin/AppFormView.vue'),
          meta: { title: 'Add app' },
        },
        {
          path: 'apps/import',
          name: 'admin-import',
          component: () => import('@/views/admin/ImportView.vue'),
          meta: { title: 'Import apps' },
        },
        {
          path: 'apps/:id/edit',
          name: 'admin-app-edit',
          component: () => import('@/views/admin/AppFormView.vue'),
          props: true,
          meta: { title: 'Edit app' },
        },
        { path: 'groups', name: 'admin-groups', component: () => import('@/views/admin/GroupsView.vue'), meta: { title: 'Groups' } },
        {
          path: 'groups/new',
          name: 'admin-group-new',
          component: () => import('@/views/admin/GroupFormView.vue'),
          meta: { title: 'New group' },
        },
        {
          path: 'groups/:name',
          name: 'admin-group-edit',
          component: () => import('@/views/admin/GroupFormView.vue'),
          props: true,
          meta: { title: 'Edit group' },
        },
        { path: 'users', name: 'admin-users', component: () => import('@/views/admin/UsersView.vue'), meta: { title: 'Users' } },
        {
          path: 'settings',
          name: 'admin-settings',
          component: () => import('@/views/admin/SettingsView.vue'),
          meta: { title: 'Settings' },
        },
      ],
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('@/views/NotFoundView.vue'),
      meta: { title: 'Page not found' },
    },
  ],
  scrollBehavior: (_to, _from, saved) => saved ?? { top: 0 },
})

function loginFor(path: string) {
  const isAdminPath = path === '/admin' || path.startsWith('/admin/')
  return { name: isAdminPath ? 'admin-login' : 'login', query: { redirect: path } }
}

router.beforeEach(async (to) => {
  if (to.meta.public) return true
  // after a sign out, go to the sign-in page instead of starting SSO again
  if ('signed_out' in to.query) {
    const target = loginFor(to.path)
    return { ...target, query: { ...target.query, signed_out: '1' } }
  }
  try {
    await ensureSession()
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) return loginFor(to.fullPath)
    // api down: let the page load and show its own error
    return true
  }
  if (to.matched.some((r) => r.meta.admin) && !isAdmin.value) return { name: 'catalog' }
  return true
})

router.afterEach((to) => {
  setTitle(to.meta.title ?? '')
})

setUnauthorizedHandler(() => {
  const path = location.pathname + location.search
  if (path.startsWith('/login') || path.startsWith('/admin/login')) return
  router.push(loginFor(path))
})

export default router
