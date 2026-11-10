<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import Avatar from '@/components/Avatar.vue'
import Icon from '@/components/Icon.vue'
import Popover from '@/components/Popover.vue'
import { isAdmin, isLocalUser, me, userName } from '@/session'

const kind = computed(() => {
  const parts = [isLocalUser.value ? 'Portal account' : 'Company SSO']
  if (isAdmin.value) parts.push('admin')
  return parts.join(', ')
})
</script>

<template>
  <Popover
    v-if="userName"
    :label="`Account menu for ${userName}`"
    align="right"
    panel-class="w-60"
    button-class="flex cursor-pointer items-center gap-1.5 rounded-full border border-line bg-white py-[3px] pr-2.5 pl-[3px] text-subtle hover:border-field"
  >
    <template #trigger="{ open }">
      <Avatar :name="userName" :size="36" />
      <Icon name="chevron" :size="16" class="transition-transform" :class="{ 'rotate-180': open }" />
    </template>
    <template #default="{ close }">
      <div class="flex items-center gap-2.5 px-2.5 pt-2 pb-2.5">
        <Avatar :name="userName" :size="32" />
        <div class="flex min-w-0 flex-col">
          <span class="truncate text-sm font-semibold">{{ userName }}</span>
          <span class="truncate text-xs text-subtle">{{ me?.user.email || kind }}</span>
        </div>
      </div>
      <div class="menu-sep" />
      <RouterLink v-if="isLocalUser" to="/account/password" role="menuitem" class="menu-item" @click="close(false)">
        <Icon name="key" :size="16" class="text-subtle" />
        Change password
      </RouterLink>
      <form method="post" action="/auth/logout" class="contents">
        <button type="submit" role="menuitem" class="menu-item">
          <Icon name="logout" :size="16" class="text-subtle" />
          Sign out
        </button>
      </form>
    </template>
  </Popover>
</template>
