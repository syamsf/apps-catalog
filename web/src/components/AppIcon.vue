<script setup lang="ts">
import { computed } from 'vue'
import { iconColor, initials, tablerURL } from '@/api'
import { settings } from '@/session'

const props = withDefaults(
  defineProps<{
    app: { id: string; name: string; icon_color: string; icon?: string; has_icon?: boolean; icon_version?: number }
    size?: number
    // preview overrides used by the app form
    src?: string
    noUpload?: boolean
  }>(),
  { size: 44 },
)

const imgSrc = computed(() => {
  if (props.src) return props.src
  if (props.noUpload || !props.app.has_icon) return ''
  return `/api/apps/${encodeURIComponent(props.app.id)}/icon?v=${props.app.icon_version ?? 0}`
})

const color = computed(() =>
  iconColor(props.app, settings.value?.icon_palette ?? ['#2251C7'], settings.value?.icon_color_mode ?? 'auto'),
)

// the glyph is a mask, so it takes the tile's text color
const glyph = computed(() => {
  if (!props.app.icon) return null
  const url = `url("${tablerURL(props.app.icon)}")`
  const size = `${Math.round(props.size * 0.58)}px`
  return { width: size, height: size, maskImage: url, WebkitMaskImage: url, maskSize: 'contain', WebkitMaskSize: 'contain' }
})

const box = computed(() => ({
  width: `${props.size}px`,
  height: `${props.size}px`,
  borderRadius: `${Math.round(props.size * 0.225)}px`,
}))
</script>

<template>
  <span
    v-if="imgSrc"
    aria-hidden="true"
    class="flex shrink-0 items-center justify-center overflow-hidden border border-line bg-white"
    :style="box"
  >
    <img :src="imgSrc" alt="" class="size-full object-contain" :style="{ padding: `${Math.round(size * 0.14)}px` }" />
  </span>
  <span
    v-else
    aria-hidden="true"
    class="flex shrink-0 items-center justify-center font-semibold text-white"
    :style="{ ...box, background: color, fontSize: `${Math.round(size * 0.35)}px` }"
  >
    <span v-if="glyph" class="bg-current [mask-repeat:no-repeat] [mask-position:center]" :style="glyph" />
    <template v-else>{{ initials(app.name) }}</template>
  </span>
</template>
