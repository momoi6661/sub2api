<template>
  <div>
    <div class="flex min-w-0 items-center gap-2">
      <span v-if="!hasSecret" class="text-sm text-gray-400 dark:text-dark-400">{{ t('admin.accounts.authProfiles.noSecret') }}</span>
      <code v-else class="min-w-0 max-w-64 break-all text-sm text-gray-700 dark:text-gray-200" :class="!visible && 'tracking-widest'">{{ visible ? secret : '••••••••' }}</code>
      <button v-if="hasSecret" type="button" class="btn btn-ghost btn-sm shrink-0" :disabled="loading" :aria-label="t(visible ? 'admin.accounts.authProfiles.hideSecret' : 'admin.accounts.authProfiles.showSecret')" :title="t(visible ? 'admin.accounts.authProfiles.hideSecret' : 'admin.accounts.authProfiles.showSecret')" :aria-pressed="visible" @click="toggle">
        <Icon :name="loading ? 'refresh' : visible ? 'eyeOff' : 'eye'" size="sm" :class="loading && 'animate-spin'" />
      </button>
    </div>
    <p v-if="error" role="alert" class="mt-1 text-xs text-red-600 dark:text-red-400">{{ error }}</p>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { readReauthSecret } from '@/api/admin/reauthProfiles'
import { extractApiErrorMessage } from '@/utils/apiError'

const props = defineProps<{ profileId: string; hasSecret: boolean }>()
const { t } = useI18n()
const secret = ref(''), visible = ref(false), loading = ref(false), error = ref('')
let controller: AbortController | null = null
function reset() {
  controller?.abort()
  controller = null
  secret.value = ''
  visible.value = false
  loading.value = false
  error.value = ''
}
async function toggle() {
  if (visible.value) { reset(); return }
  if (loading.value || !props.hasSecret) return
  const request = new AbortController()
  controller = request
  loading.value = true
  error.value = ''
  try {
    const value = await readReauthSecret(props.profileId, request.signal)
    if (!request.signal.aborted) { secret.value = value; visible.value = true }
  } catch (e) {
    if (!request.signal.aborted) error.value = extractApiErrorMessage(e, t('admin.accounts.authProfiles.failed'))
  } finally {
    if (controller === request) { loading.value = false; controller = null }
  }
}
watch(() => [props.profileId, props.hasSecret], reset)
onBeforeUnmount(reset)
</script>
