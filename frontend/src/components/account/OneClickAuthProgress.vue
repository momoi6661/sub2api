<template>
  <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600" role="status" aria-live="polite">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <div class="flex min-w-0 items-center gap-2">
        <Icon :name="running ? 'refresh' : status === 'failed' ? 'x' : 'check'" size="md" :class="running ? 'animate-spin text-primary-500' : status === 'failed' ? 'text-red-500' : 'text-green-500'" />
        <span class="min-w-0 break-all font-medium text-gray-900 dark:text-white">{{ title || t('admin.accounts.oneClickAuth.title') }}</span>
      </div>
      <span class="text-xs tabular-nums text-gray-500 dark:text-dark-400">{{ t('admin.accounts.oneClickAuth.elapsed', { seconds: elapsed }) }}</span>
    </div>
    <p class="mt-2 text-sm" :class="status === 'failed' ? 'text-red-600 dark:text-red-400' : 'text-gray-600 dark:text-dark-300'">
      {{ t(`admin.accounts.oneClickAuth.${status === 'running' ? stage : status}`) }}
    </p>
    <ol class="mt-3 grid grid-cols-2 gap-2 text-xs sm:grid-cols-5" :aria-label="t('admin.accounts.oneClickAuth.progress')">
      <li v-for="(item, index) in stages" :key="item" class="flex items-center gap-1.5" :class="index <= activeIndex ? 'text-primary-600 dark:text-primary-400' : 'text-gray-400 dark:text-dark-500'" :aria-current="index === activeIndex && running ? 'step' : undefined">
        <Icon v-if="index < activeIndex || status === 'succeeded'" name="check" size="sm" />
        <span v-else class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full border border-current tabular-nums">{{ index + 1 }}</span>
        <span>{{ t(`admin.accounts.oneClickAuth.step${index + 1}`) }}</span>
      </li>
    </ol>
    <p v-if="running" class="mt-3 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.accounts.oneClickAuth.progressHint') }}</p>
    <p v-if="error" class="mt-3 break-words text-sm text-red-600 dark:text-red-400">{{ error }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { AuthorizationStage } from '@/api/admin/reauthProfiles'

const props = defineProps<{
  title?: string; stage: AuthorizationStage; status: 'running' | 'succeeded' | 'failed' | 'ready'
  startedAt: number; finishedAt?: number; error?: string
}>()
const { t } = useI18n()
const now = ref(Date.now())
const running = computed(() => props.status === 'running')
const elapsed = computed(() => Math.max(0, Math.floor(((props.finishedAt || now.value) - props.startedAt) / 1000)))
const stages: AuthorizationStage[] = ['configuration', 'connecting', 'browser_authorization', 'verifying', 'saving']
const activeIndex = computed(() => props.status === 'succeeded' ? 4 : props.stage === 'authorization_file' ? 2 : Math.max(0, stages.indexOf(props.stage)))
let timer: ReturnType<typeof setInterval> | undefined
watch(running, value => {
  if (timer) clearInterval(timer)
  now.value = Date.now()
  timer = value ? setInterval(() => { now.value = Date.now() }, 1000) : undefined
}, { immediate: true })
onBeforeUnmount(() => { if (timer) clearInterval(timer) })
</script>
