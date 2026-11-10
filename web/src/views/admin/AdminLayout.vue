<script setup lang="ts">
import { RouterLink, RouterView, useRoute } from 'vue-router'
import AccountMenu from '@/components/AccountMenu.vue'
import Icon from '@/components/Icon.vue'
import Logo from '@/components/Logo.vue'

const route = useRoute()

const nav = [
  { to: '/admin', label: 'Dashboard', icon: 'dashboard', exact: true },
  { to: '/admin/apps', label: 'Apps', icon: 'grid' },
  { to: '/admin/groups', label: 'Groups', icon: 'group' },
  { to: '/admin/users', label: 'Users', icon: 'user' },
  { to: '/admin/settings', label: 'Settings', icon: 'sliders' },
] as const

const active = (item: { to: string; exact?: boolean }) =>
  item.exact ? route.path === item.to || route.path === `${item.to}/` : route.path.startsWith(item.to)
</script>

<template>
  <div class="min-h-dvh">
    <header class="border-b border-line bg-white">
      <div class="mx-auto flex max-w-[1440px] flex-wrap items-center gap-x-4 gap-y-3 px-4 py-3.5 sm:px-8">
        <Logo to="/admin" />
        <span class="rounded-md bg-ink px-2.5 py-1 text-xs font-semibold tracking-[0.04em] text-white">ADMIN</span>
        <div class="ml-auto flex flex-wrap items-center gap-2.5">
          <RouterLink to="/" class="btn btn-secondary">View catalog</RouterLink>
          <AccountMenu />
        </div>
      </div>
    </header>
    <div class="mx-auto flex max-w-[1440px] flex-col gap-7 px-4 pt-6 pb-24 sm:px-8 md:flex-row md:items-start">
      <nav aria-label="Admin sections" class="flex flex-row flex-wrap gap-0.5 md:w-[200px] md:shrink-0 md:flex-col">
        <RouterLink
          v-for="item in nav"
          :key="item.to"
          :to="item.to"
          :aria-current="active(item) ? 'page' : undefined"
          class="flex min-h-10 items-center gap-2.5 rounded-lg px-3 text-sm no-underline"
          :class="active(item) ? 'bg-accent-soft font-semibold text-accent-ink' : 'font-medium text-ink hover:bg-chip hover:text-ink'"
        >
          <Icon :name="item.icon" :class="active(item) ? '' : 'text-muted'" />
          {{ item.label }}
        </RouterLink>
      </nav>
      <main class="flex min-w-0 flex-1 flex-col gap-5">
        <RouterView />
      </main>
    </div>
  </div>
</template>
