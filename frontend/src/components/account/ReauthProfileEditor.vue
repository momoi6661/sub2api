<template>
  <BaseDialog :show="show" :title="t('admin.accounts.authProfiles.edit')" @close="emit('close')">
    <form id="reauth-profile-form" class="space-y-4" @submit.prevent="save">
      <p class="input-hint">{{ t('admin.accounts.authProfiles.pending') }}</p>
      <div><label for="auth-profile-name" class="input-label">{{ t('admin.accounts.authProfiles.name') }}</label><input id="auth-profile-name" v-model="form.name" class="input" required maxlength="120" /></div>
      <label class="flex items-center gap-2"><input v-model="form.enabled" type="checkbox" />{{ t('admin.accounts.authProfiles.enabled') }}</label>
      <div><label for="auth-profile-mailbox" class="input-label">{{ t('admin.accounts.authProfiles.mailbox') }}</label><input id="auth-profile-mailbox" v-model="form.mailbox_address" class="input" type="email" :required="form.mailbox_mode !== 'api' || form.api_result_type !== 'auth_file'" /></div>
      <p v-if="form.mailbox_mode === 'imap' || form.api_result_type !== 'auth_file'" class="input-hint">{{ t('admin.accounts.authProfiles.mailboxHint') }}</p>
      <div>
        <label for="auth-profile-mode" class="input-label">{{ t('admin.accounts.authProfiles.mode') }}</label>
        <Select id="auth-profile-mode" v-model="form.mailbox_mode" :options="mailboxModeOptions" :searchable="false" :disabled="saving || testing" :aria-label="t('admin.accounts.authProfiles.mode')" />
      </div>
      <div v-if="form.mailbox_mode === 'imap'" class="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div><label for="auth-profile-host" class="input-label">{{ t('admin.accounts.authProfiles.host') }}</label><input id="auth-profile-host" v-model="form.imap_host" class="input" required /></div>
        <div><label for="auth-profile-port" class="input-label">{{ t('admin.accounts.authProfiles.port') }}</label><input id="auth-profile-port" v-model.number="form.imap_port" class="input" type="number" min="1" max="65535" required /></div>
      </div>
      <div v-else class="space-y-3">
        <label for="auth-profile-result" class="input-label">{{ t('admin.accounts.authProfiles.resultType') }}</label>
        <Select id="auth-profile-result" v-model="form.api_result_type" :options="resultTypeOptions" :searchable="false" :disabled="saving || testing" :aria-label="t('admin.accounts.authProfiles.resultType')" />
        <p class="input-hint">{{ t(form.api_result_type === 'auth_file' ? 'admin.accounts.authProfiles.fileHint' : 'admin.accounts.authProfiles.codeHint') }}</p>
        <label for="auth-profile-script" class="input-label">{{ t('admin.accounts.authProfiles.script') }}</label>
        <textarea id="auth-profile-script" v-model="form.code_script" class="input min-h-48 font-mono text-sm" spellcheck="false" maxlength="16384" :placeholder="scriptExample" />
        <p class="input-hint">{{ t(form.has_code_script ? 'admin.accounts.authProfiles.scriptKeep' : 'admin.accounts.authProfiles.scriptHint') }}</p>
        <ReauthScriptRules />
        <label for="auth-profile-test-email" class="input-label">{{ t('admin.accounts.authProfiles.testEmail') }}</label>
        <input id="auth-profile-test-email" v-model="testEmail" type="email" class="input" />
        <button type="button" class="btn btn-secondary" :disabled="testing" @click="testScript">{{ t('admin.accounts.authProfiles.testCode') }}</button>
        <p v-if="testResult" role="status" class="text-sm text-green-600">{{ t('admin.accounts.authProfiles.testSuccess') }}</p>
      </div>
      <div>
        <div v-if="show && form.id && form.has_secret" class="mb-3 rounded-lg bg-gray-50 p-3 dark:bg-dark-700">
          <p class="mb-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.accounts.authProfiles.currentSecret') }}</p>
          <ReauthProfileSecret :profile-id="form.id" :has-secret="form.has_secret" />
        </div>
        <label for="auth-profile-secret" class="input-label">{{ t('admin.accounts.authProfiles.secret') }}</label>
        <div class="relative">
          <input id="auth-profile-secret" v-model="form.secret" class="input pr-12" :type="showSecret ? 'text' : 'password'" autocomplete="new-password" />
          <button type="button" class="absolute inset-y-0 right-0 flex w-11 items-center justify-center rounded-r-lg text-gray-500 hover:text-gray-900 focus-visible:ring-2 focus-visible:ring-primary-500 dark:hover:text-white" :aria-label="t(showSecret ? 'admin.accounts.authProfiles.hideSecret' : 'admin.accounts.authProfiles.showSecret')" :aria-pressed="showSecret" @click="showSecret = !showSecret"><Icon :name="showSecret ? 'eyeOff' : 'eye'" size="md" /></button>
        </div>
        <p class="input-hint">{{ t(form.has_secret ? 'admin.accounts.authProfiles.secretKeep' : 'admin.accounts.authProfiles.secretHint') }}</p>
        <p v-if="form.mailbox_mode === 'imap' && /(^|\.)gmail\.com$/i.test(form.imap_host)" class="input-hint">{{ t('admin.accounts.authProfiles.gmailPasswordHint') }} <a href="https://support.google.com/mail/answer/185833" target="_blank" rel="noopener noreferrer" class="text-primary-600 underline dark:text-primary-400">{{ t('admin.accounts.authProfiles.gmailPasswordHelp') }}</a></p>
      </div>
      <div v-if="form.mailbox_mode === 'imap'" class="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div><label for="auth-profile-folder" class="input-label">{{ t('admin.accounts.authProfiles.folder') }}</label><input id="auth-profile-folder" v-model="form.folder" class="input" /></div>
        <div><label for="auth-profile-sender" class="input-label">{{ t('admin.accounts.authProfiles.sender') }}</label><input id="auth-profile-sender" v-model="form.sender_filter" class="input" /></div>
      </div>
      <div v-if="form.mailbox_mode === 'imap'"><label for="auth-profile-pattern" class="input-label">{{ t('admin.accounts.authProfiles.pattern') }}</label><input id="auth-profile-pattern" v-model="form.code_pattern" class="input font-mono" required /><p class="input-hint">{{ t('admin.accounts.authProfiles.isolation') }}</p></div>
      <div v-if="form.mailbox_mode === 'imap'"><label for="auth-profile-wait" class="input-label">{{ t('admin.accounts.authProfiles.waitSeconds') }}</label><input id="auth-profile-wait" v-model.number="form.mailbox_wait_seconds" class="input" type="number" min="30" max="90" step="1" required /><p class="input-hint">{{ t('admin.accounts.authProfiles.waitHint') }}</p></div>
      <div v-if="form.mailbox_mode === 'imap' || form.api_result_type === 'code'"><label for="auth-profile-resend" class="input-label">{{ t('admin.accounts.authProfiles.resendSeconds') }}</label><input id="auth-profile-resend" v-model.number="form.email_resend_seconds" class="input" type="number" min="10" max="90" step="1" required /><p class="input-hint">{{ t('admin.accounts.authProfiles.resendHint') }}</p></div>
      <div><label class="input-label">{{ t('admin.accounts.authProfiles.proxy') }}</label><div class="max-h-48 space-y-2 overflow-y-auto rounded-lg border border-gray-200 p-3 dark:border-dark-600"><label v-for="proxy in proxies" :key="proxy.id" class="flex items-center gap-2 text-sm"><input v-model="form.proxy_ids" type="checkbox" :value="proxy.id" />{{ proxy.name }}</label><p v-if="!proxies.length" class="input-hint">{{ t('admin.accounts.authProfiles.noProxies') }}</p></div><p class="input-hint">{{ t('admin.accounts.authProfiles.proxyHint') }}</p></div>
      <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p><p v-if="errorCode" class="text-sm text-red-600">{{ errorCode }} · {{ t(errorRetryable ? 'admin.accounts.authProfiles.retryable' : 'admin.accounts.authProfiles.notRetryable') }}</p>
    </form>
    <template #footer><button class="btn btn-secondary" :disabled="saving" @click="emit('close')">{{ t('common.cancel') }}</button><button form="reauth-profile-form" type="submit" class="btn btn-primary" :disabled="saving">{{ t('common.save') }}</button></template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch, onBeforeUnmount, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import ReauthProfileSecret from './ReauthProfileSecret.vue'
import ReauthScriptRules from './ReauthScriptRules.vue'
import { saveReauthProfile, testReauthScript, type ReauthProfile } from '@/api/admin/reauthProfiles'
import { getAll } from '@/api/admin/proxies'
import type { Proxy } from '@/types'

const props = defineProps<{ show: boolean; profile?: ReauthProfile | null }>()
const emit = defineEmits<{ close: []; saved: [profile: ReauthProfile] }>()
const { t } = useI18n()
const mailboxModeOptions = computed(() => [{ value: 'imap', label: 'IMAP (TLS)' }, { value: 'api', label: t('admin.accounts.authProfiles.customCode') }])
const resultTypeOptions = computed(() => [
  { value: 'code', label: t('admin.accounts.authProfiles.codeResult') },
  { value: 'auth_file', label: t('admin.accounts.authProfiles.fileResult') }
])
const defaults = (): ReauthProfile => ({ id: '', name: '', enabled: true, mailbox_mode: 'imap', api_result_type: 'code', mailbox_address: '', imap_host: '', imap_port: 993, imap_username: '', api_url: '', code_script: '', has_code_script: false, folder: 'INBOX', sender_filter: '', mailbox_wait_seconds: 60, email_resend_seconds: 30, code_pattern: '(\\d{6})', proxy_id: null, proxy_ids: [], secret: '', has_secret: false })
const form = ref(defaults())
const proxies = ref<Proxy[]>([])
const saving = ref(false)
const showSecret = ref(false)
const error = ref(''), errorCode = ref(''), errorRetryable = ref(false)
const testEmail = ref(''), testResult = ref(''), testing = ref(false)
let testAbort: AbortController | null = null
const scriptExample = computed(() => `// context: loginEmail, since, mailboxAddress, secret
// Set your endpoint and request options in this code.
// Filter messages by recipient and date in your request.
const response = await fetch('https://your-mail-service.example/messages', {
  headers: { Authorization: 'Bearer ' + context.secret },
  credentials: 'omit'
});
if (!response.ok) throw new Error('Mailbox request failed');
const data = await response.json();
${form.value.api_result_type === 'auth_file' ? 'return { type: \'auth_file\', auth: data };' : 'return { type: \'code\', code: String(data.code) };'}`)
async function testScript() {
  if (testing.value) return
  testing.value = true; error.value = ''; errorCode.value = ''; errorRetryable.value = false; testResult.value = ''
  const controller = new AbortController(); testAbort = controller
  try {
    await testReauthScript(form.value, testEmail.value, controller.signal)
    if (!controller.signal.aborted) testResult.value = 'ok'
  } catch (e: any) { if (!controller.signal.aborted) { error.value = e?.message || t('admin.accounts.authProfiles.failed'); errorCode.value = e?.reason || e?.response?.data?.reason || e?.code || 'REQUEST_FAILED'; errorRetryable.value = (e?.metadata || e?.response?.data?.metadata)?.retryable === 'true' } }
  finally { testing.value = false }
}
onBeforeUnmount(() => testAbort?.abort())
watch(() => props.show, async show => {
  showSecret.value = false
  testAbort?.abort(); testResult.value = ''; testEmail.value = ''
  if (!show) { form.value.secret = ''; form.value.code_script = ''; return }
  form.value = props.profile ? { ...props.profile, proxy_ids: [...(props.profile.proxy_ids || (props.profile.proxy_id ? [props.profile.proxy_id] : []))], proxy_id: null, mailbox_wait_seconds: props.profile.mailbox_wait_seconds || 60, email_resend_seconds: props.profile.email_resend_seconds || 30, api_result_type: props.profile.api_result_type || 'code', secret: '', code_script: '' } : defaults()
  error.value = ''; errorCode.value = ''
  try { proxies.value = await getAll() } catch (e: any) { error.value = e?.message || t('admin.accounts.authProfiles.failed') }
})
async function save() {
  if (saving.value) return
  saving.value = true
  error.value = ''
  try { emit('saved', await saveReauthProfile({ ...form.value, imap_username: form.value.mailbox_address.trim() })); form.value.secret = ''; form.value.code_script = ''; emit('close') }
  catch (e: any) { error.value = e?.message || t('admin.accounts.authProfiles.failed') }
  finally { saving.value = false }
}
</script>
