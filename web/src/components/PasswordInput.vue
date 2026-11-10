<script setup lang="ts">
import { ref } from 'vue'
import Icon from '@/components/Icon.vue'

defineProps<{ id: string; autocomplete: string; invalid?: boolean; name?: string }>()
const model = defineModel<string>({ required: true })
const shown = ref(false)
</script>

<template>
  <div class="relative">
    <input
      :id="id"
      v-model="model"
      :type="shown ? 'text' : 'password'"
      :name="name"
      :autocomplete="autocomplete"
      spellcheck="false"
      class="field pr-12"
      :aria-invalid="invalid || undefined"
    />
    <button
      type="button"
      class="icon-btn absolute top-1.5 right-1.5"
      :aria-label="shown ? 'Hide password' : 'Show password'"
      :aria-pressed="shown"
      @click="shown = !shown"
    >
      <Icon :name="shown ? 'eye-off' : 'eye'" :size="18" />
    </button>
  </div>
</template>
