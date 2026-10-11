<template>
  <div class="mt-2 max-w-full whitespace-normal break-words text-xs leading-5">
    <template v-if="expiry">
      <div class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.subscriptionExpiryDate') }} {{ dateLabel }}</div>
      <div :class="expired ? 'text-red-600 dark:text-red-400' : 'text-emerald-600 dark:text-emerald-400'">
        {{ expired ? t('admin.accounts.subscriptionExpired') : t('admin.accounts.subscriptionRemainingDays', { days: remainingDays }) }}
      </div>
    </template>
    <div v-else class="text-gray-400 dark:text-gray-500">{{ t('admin.accounts.subscriptionExpiryUnavailable') }}</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useNow } from '@vueuse/core'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ expiresAt?: string }>()
const { t } = useI18n()
const now = useNow({ interval: 60_000 })
const expiry = computed(() => {
  if (!props.expiresAt) return null
  const date = new Date(props.expiresAt)
  return Number.isFinite(date.getTime()) ? date : null
})
const dateLabel = computed(() => {
  const date = expiry.value
  if (!date) return ''
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
})
const expired = computed(() => !!expiry.value && expiry.value.getTime() <= now.value.getTime())
const remainingDays = computed(() => Math.max(0, Math.ceil(((expiry.value?.getTime() || 0) - now.value.getTime()) / 86_400_000)))
</script>
