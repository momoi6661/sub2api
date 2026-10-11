<template>
  <div ref="rootRef" class="mt-2 max-w-full whitespace-normal break-words text-xs leading-5">
    <template v-if="expiry">
      <div class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.subscriptionExpiryDate') }} {{ dateLabel }}</div>
      <div :class="expired ? 'text-red-600 dark:text-red-400' : 'text-emerald-600 dark:text-emerald-400'">
        {{ expired ? t('admin.accounts.subscriptionExpired') : t('admin.accounts.subscriptionRemainingDays', { days: remainingDays }) }}
      </div>
    </template>
    <div v-else class="text-gray-400 dark:text-gray-500">
      {{ t(loading ? 'admin.accounts.subscriptionExpiryLoading' : !enabled && !queried ? 'admin.accounts.subscriptionExpiryDisabled' : 'admin.accounts.subscriptionExpiryUnavailable') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useNow } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { getSubscription } from '@/api/admin/accounts'

const props = withDefaults(defineProps<{ accountId: number; enabled?: boolean; refreshKey?: number }>(), { enabled: true })
const { t } = useI18n()
const now = useNow({ interval: 60_000 })
const rootRef = ref<HTMLElement | null>(null)
const expiresAt = ref('')
const loading = ref(false)
let visible = false
let observer: IntersectionObserver | null = null
let request: AbortController | null = null
const queried = ref(false)
let pendingForce = (props.refreshKey ?? 0) > 0

async function loadSubscription(force = false) {
  if (!props.enabled || !visible || request || props.accountId < 1) return
  const currentRequest = new AbortController()
  request = currentRequest
  loading.value = true
  try {
    const result = await getSubscription(props.accountId, force, currentRequest.signal)
    if (request === currentRequest) expiresAt.value = result.subscription_expires_at
  } catch {
    // Keep a previously queried date during a transient request failure.
  } finally {
    if (request === currentRequest) {
      queried.value = true
      pendingForce = false
      request = null
      loading.value = false
    }
  }
}

onMounted(() => {
  if (typeof IntersectionObserver === 'undefined') {
    visible = true
    void loadSubscription(pendingForce)
    return
  }
  observer = new IntersectionObserver((entries) => {
    visible = entries.some(entry => entry.isIntersecting)
    if (visible && (!queried.value || pendingForce)) void loadSubscription(pendingForce)
  }, { rootMargin: '100px 0px' })
  if (rootRef.value) observer.observe(rootRef.value)
})
watch(() => [props.accountId, props.refreshKey], (next, previous) => {
  const identityChanged = next[0] !== previous[0]
  request?.abort()
  request = null
  if (identityChanged) expiresAt.value = ''
  queried.value = false
  pendingForce = !identityChanged
  loading.value = false
  void loadSubscription(pendingForce)
})
watch(() => props.enabled, (enabled) => {
  if (enabled) {
    pendingForce = true
    void loadSubscription(true)
  } else {
    request?.abort()
    request = null
    loading.value = false
  }
})
onBeforeUnmount(() => {
  observer?.disconnect()
  request?.abort()
  request = null
})

const expiry = computed(() => {
  if (!expiresAt.value) return null
  const date = new Date(expiresAt.value)
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
