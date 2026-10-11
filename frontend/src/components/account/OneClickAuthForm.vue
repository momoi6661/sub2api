<template>
  <form id="new-account-one-click-auth" class="space-y-4" @submit.prevent="authorize">
    <p class="text-sm text-gray-500 dark:text-dark-400">{{ t(accountID ? 'admin.accounts.authorizationTasks.existingHint' : 'admin.accounts.oneClickAuth.createHint') }}</p>
    <div>
      <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
        <label for="new-account-auth-profile" class="input-label mb-0">{{ t('admin.accounts.oneClickAuth.profile') }}</label>
        <button type="button" class="btn btn-ghost btn-sm" :disabled="busy" @click="showEditor = true">{{ t('admin.accounts.authProfiles.create') }}</button>
      </div>
      <ReauthProfileSelect id="new-account-auth-profile" v-model="profileID" :profiles="profiles" :disabled="loading || busy" />
      <p v-if="loadError" role="alert" class="mt-2 text-sm text-red-600">{{ loadError }} <button type="button" class="underline" :disabled="loading" @click="loadProfiles">{{ t('common.refresh') }}</button></p>
      <p v-else-if="!loading && !profiles.length" class="input-hint">{{ t('admin.accounts.authProfiles.empty') }}</p>
    </div>
    <div>
      <label for="new-account-auth-email" class="input-label">{{ t('admin.accounts.authProfiles.loginEmail') }}</label>
      <input id="new-account-auth-email" v-model="loginEmail" class="input" type="email" required maxlength="254" autocomplete="email" :disabled="busy" placeholder="name@example.com" />
    </div>
    <div>
      <label for="new-account-workspace-index" class="input-label">{{ t('admin.accounts.oneClickAuth.workspaceIndex') }}</label>
      <input id="new-account-workspace-index" v-model.number="workspaceIndex" class="input" type="number" min="1" step="1" required :disabled="busy" />
      <p class="input-hint">{{ t('admin.accounts.oneClickAuth.workspaceIndexHint') }}</p>
    </div>
    <p class="input-hint">{{ t('admin.accounts.authProfiles.browser') }}</p>
    <OneClickAuthProgress v-if="taskStartedAt && !accountID" :stage="submitting ? 'saving' : taskStage" :status="busy ? 'running' : error ? 'failed' : 'ready'" :started-at="taskStartedAt" :finished-at="busy ? undefined : taskFinishedAt" :error="error && errorCode ? `${errorCode} · ${t(errorRetryable ? 'admin.accounts.authProfiles.retryable' : 'admin.accounts.authProfiles.notRetryable')}` : undefined" />
    <p v-if="authorization" role="status" class="text-sm text-green-600 dark:text-green-400">{{ t('admin.accounts.oneClickAuth.readyToCreate') }}</p>
    <div v-if="error" role="alert" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-400">
      <p>{{ error }}</p>
      <p v-if="errorCode" class="mt-1">{{ errorCode }} · {{ t(errorRetryable ? 'admin.accounts.authProfiles.retryable' : 'admin.accounts.authProfiles.notRetryable') }}</p>
    </div>
    <button type="submit" class="btn btn-primary w-full sm:w-auto" :disabled="busy || loading || !selectedEnabled || !loginEmail.trim()">
      <Icon v-if="busy" name="refresh" size="md" class="mr-2 animate-spin" />
      {{ t(busy ? 'admin.accounts.oauth.verifying' : accountID ? 'admin.accounts.authorizationTasks.submit' : authorization ? 'common.create' : 'admin.accounts.oneClickAuth.authorizeAndCreate') }}
    </button>
    <ReauthProfileEditor :show="showEditor" @close="showEditor = false" @saved="profileCreated" />
  </form>
</template>

<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ReauthProfileSelect from './ReauthProfileSelect.vue'
import ReauthProfileEditor from './ReauthProfileEditor.vue'
import OneClickAuthProgress from './OneClickAuthProgress.vue'
import { authorizeNewAccount, listReauthProfiles, submitAuthorizationTasks, type AuthorizationTask, type AuthorizationStage, type NewAccountAuthorization, type ReauthProfile } from '@/api/admin/reauthProfiles'
import { extractApiErrorCode, extractApiErrorMessage, extractApiErrorMetadata } from '@/utils/apiError'

const props = withDefaults(defineProps<{ proxyID?: number | null; submitting?: boolean; accountID?: number; initialProfileID?: string; initialLoginEmail?: string; initialWorkspaceIndex?: number }>(), { proxyID: null, submitting: false, initialProfileID: '', initialLoginEmail: '', initialWorkspaceIndex: 1 })
const emit = defineEmits<{ authorized: [payload: NewAccountAuthorization, profileID: string, loginEmail: string, workspaceIndex: number]; submitted: [task: AuthorizationTask] }>()
const { t } = useI18n()
const profiles = ref<ReauthProfile[]>([])
const profileID = ref(props.initialProfileID), loginEmail = ref(props.initialLoginEmail)
const workspaceIndex = ref(props.initialWorkspaceIndex)
const loading = ref(false), authorizing = ref(false), showEditor = ref(false)
const loadError = ref(''), error = ref(''), errorCode = ref(''), errorRetryable = ref(false)
const authorization = shallowRef<NewAccountAuthorization | null>(null)
const taskStartedAt = ref(0), taskFinishedAt = ref<number | undefined>()
const taskStage = ref<AuthorizationStage>('preparing')
const busy = computed(() => authorizing.value || props.submitting)
const selectedEnabled = computed(() => profiles.value.some(p => p.id === profileID.value && p.enabled))
let controller: AbortController | null = null
let disposed = false

watch([profileID, loginEmail, workspaceIndex, () => props.proxyID], () => {
  controller?.abort()
  authorization.value = null
  error.value = ''; errorCode.value = ''
  taskStartedAt.value = 0
})
watch(() => props.submitting, value => { if (!value && taskStartedAt.value) taskFinishedAt.value = Date.now() })

async function loadProfiles() {
  loading.value = true; loadError.value = ''
  try { const result = await listReauthProfiles(); if (!disposed) profiles.value = result }
  catch (e) { if (!disposed) loadError.value = extractApiErrorMessage(e, t('admin.accounts.authProfiles.failed')) }
  finally { loading.value = false }
}
async function profileCreated(profile: ReauthProfile) {
  showEditor.value = false
  authorization.value = null
  await loadProfiles()
  profileID.value = profile.id
}
async function authorize() {
  if (busy.value || !selectedEnabled.value || !loginEmail.value.trim()) return
  const selectedProfile = profileID.value, email = loginEmail.value.trim()
  const taskController = new AbortController()
  controller = taskController
  taskStartedAt.value = Date.now(); taskFinishedAt.value = undefined; taskStage.value = 'preparing'
  authorizing.value = true; error.value = ''; errorCode.value = ''
  try {
    if (props.accountID) {
      const [task] = await submitAuthorizationTasks([props.accountID], { profile_id: selectedProfile, login_email: email, workspace_index: workspaceIndex.value })
      if (!disposed) emit('submitted', task)
      return
    }
    if (!authorization.value) {
      const result = await authorizeNewAccount(selectedProfile, email, props.proxyID, taskController.signal, progress => {
        if (!disposed && !taskController.signal.aborted) taskStage.value = progress.stage
      }, workspaceIndex.value)
      if (disposed || taskController.signal.aborted) return
      authorization.value = result
    }
    taskStage.value = 'verifying'
    if (!disposed && !taskController.signal.aborted) {
      // Retain the result if creation fails, so retrying does not repeat OAuth.
      emit('authorized', { ...authorization.value, credentials: { ...authorization.value.credentials } }, selectedProfile, email, workspaceIndex.value)
    }
  } catch (e) {
    if (!disposed && !taskController.signal.aborted) {
      error.value = extractApiErrorMessage(e, t('admin.accounts.authProfiles.failed'))
      errorCode.value = extractApiErrorCode(e) || ''
      errorRetryable.value = extractApiErrorMetadata(e)?.retryable === 'true'
    }
  } finally {
    authorizing.value = false
    taskFinishedAt.value = Date.now()
    if (controller === taskController) controller = null
  }
}
onMounted(loadProfiles)
onBeforeUnmount(() => { disposed = true; controller?.abort(); authorization.value = null })
</script>
