<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div class="min-w-0">
            <h2 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('admin.accounts.authProfiles.title') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.accounts.authProfiles.description') }}</p>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            <button type="button" class="btn btn-secondary" :disabled="loading" :aria-label="t('common.refresh')" :title="t('common.refresh')" @click="load"><Icon name="refresh" size="md" :class="loading && 'animate-spin'" /></button>
            <button type="button" class="btn btn-primary" @click="edit(null)"><Icon name="plus" size="md" class="mr-2" />{{ t('admin.accounts.authProfiles.create') }}</button>
          </div>
        </div>
      </template>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="w-full sm:w-80"><SearchInput v-model="searchQuery" :placeholder="t('admin.accounts.authProfiles.search')" :aria-label="t('admin.accounts.authProfiles.search')" /></div>
          <div class="w-full sm:w-40"><Select v-model="mode" :options="modeOptions" :aria-label="t('admin.accounts.authProfiles.mode')" /></div>
          <div class="w-full sm:w-36"><Select v-model="status" :options="statusOptions" :aria-label="t('common.status')" /></div>
          <button v-if="hasFilters" type="button" class="btn btn-ghost btn-sm" @click="clearFilters">{{ t('admin.accounts.authProfiles.clearFilters') }}</button>
          <span class="text-sm tabular-nums text-gray-500 dark:text-dark-400 sm:ml-auto">{{ t('admin.accounts.authProfiles.count', { count: filteredProfiles.length }) }}</span>
        </div>
        <p v-if="error" role="alert" class="mt-3 rounded-lg bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-400">{{ error }}</p>
      </template>
      <template #table>
        <DataTable :columns="columns" :data="pagedProfiles" :loading="loading && !profiles.length" row-key="id" :expandable-actions="false">
          <template #cell-name="{ row }"><span class="block max-w-64 truncate font-medium text-gray-900 dark:text-white" :title="row.name">{{ row.name }}</span></template>
          <template #cell-mailbox_address="{ row }">
            <div class="max-w-80">
              <p class="truncate text-gray-700 dark:text-gray-200" :title="row.mailbox_address">{{ row.mailbox_address || '—' }}</p>
              <p class="mt-0.5 truncate text-xs text-gray-500 dark:text-dark-400" :title="endpoint(row)">{{ endpoint(row) }}</p>
            </div>
          </template>
          <template #cell-mailbox_mode="{ row }">
            <div class="space-y-1"><span class="badge badge-gray">{{ row.mailbox_mode === 'imap' ? 'IMAP · TLS' : t('admin.accounts.authProfiles.customCode') }}</span><p class="text-xs text-gray-500 dark:text-dark-400">{{ t(row.mailbox_mode === 'api' && row.api_result_type === 'auth_file' ? 'admin.accounts.authProfiles.fileResult' : 'admin.accounts.authProfiles.codeResult') }}</p></div>
          </template>
          <template #cell-secret="{ row }"><ReauthProfileSecret :key="`${row.id}:${credentialRevision}`" :profile-id="row.id" :has-secret="row.has_secret" /></template>
          <template #cell-proxy_ids="{ row }"><span class="text-sm text-gray-500 dark:text-dark-400">{{ proxyCount(row) ? t('admin.accounts.authProfiles.proxyCount', { count: proxyCount(row) }) : t('admin.accounts.authProfiles.accountProxy') }}</span></template>
          <template #cell-enabled="{ row }"><span :class="['badge', row.enabled ? 'badge-success' : 'badge-gray']">{{ t(row.enabled ? 'common.enabled' : 'common.disabled') }}</span></template>
          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1"><button type="button" class="btn btn-ghost btn-sm" @click.stop="edit(row)">{{ t('common.edit') }}</button><button type="button" class="btn btn-ghost btn-sm text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20" @click.stop="askDelete(row)">{{ t('common.delete') }}</button></div>
          </template>
          <template #empty>
            <div class="flex flex-col items-center gap-3 py-8">
              <Icon :name="hasFilters ? 'search' : 'inbox'" size="xl" class="text-gray-400 dark:text-dark-500" />
              <p class="text-sm text-gray-500 dark:text-dark-400">{{ error || t(hasFilters ? 'admin.accounts.authProfiles.noMatches' : 'admin.accounts.authProfiles.empty') }}</p>
              <button v-if="hasFilters" type="button" class="btn btn-secondary btn-sm" @click="clearFilters">{{ t('admin.accounts.authProfiles.clearFilters') }}</button>
              <button v-else-if="!error" type="button" class="btn btn-primary btn-sm" @click="edit(null)">{{ t('admin.accounts.authProfiles.create') }}</button>
            </div>
          </template>
        </DataTable>
      </template>
      <template #pagination><Pagination :total="filteredProfiles.length" v-model:page="page" v-model:page-size="pageSize" :page-size-options="[20, 50, 100]" /></template>
    </TablePageLayout>
    <ReauthProfileEditor :show="editing" :profile="current" @close="editing = false" @saved="load" />
    <BaseDialog :show="!!deleting" :title="t('common.delete')" @close="!removing && (deleting = null)">
      <p>{{ t('admin.accounts.authProfiles.deleteConfirm', { name: deleting?.name }) }}</p><p v-if="deleteError" class="mt-3 text-red-600" role="alert">{{ deleteError }}</p>
      <template #footer><button class="btn btn-secondary" :disabled="removing" @click="deleting = null">{{ t('common.cancel') }}</button><button class="btn btn-danger" :disabled="removing" @click="remove">{{ t('common.delete') }}</button></template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import ReauthProfileEditor from '@/components/account/ReauthProfileEditor.vue'
import ReauthProfileSecret from '@/components/account/ReauthProfileSecret.vue'
import { listReauthProfiles, deleteReauthProfile, type ReauthProfile } from '@/api/admin/reauthProfiles'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const route = useRoute(), router = useRouter()
const profiles = ref<ReauthProfile[]>([])
const loading = ref(false), editing = ref(false), removing = ref(false)
const current = ref<ReauthProfile | null>(null), deleting = ref<ReauthProfile | null>(null)
const error = ref(''), deleteError = ref('')
const pageSize = ref(20)
const credentialRevision = ref(0)
function queryValue(key: string) { return typeof route.query[key] === 'string' ? route.query[key] as string : '' }
function setFilter(key: string, value: string) { void router.replace({ query: { ...route.query, [key]: value || undefined, page: undefined } }) }
const searchQuery = computed({ get: () => queryValue('q'), set: value => setFilter('q', value) })
const mode = computed({ get: () => ['imap', 'api'].includes(queryValue('mode')) ? queryValue('mode') : '', set: value => setFilter('mode', value) })
const status = computed({ get: () => ['enabled', 'disabled'].includes(queryValue('status')) ? queryValue('status') : '', set: value => setFilter('status', value) })
const hasFilters = computed(() => !!(searchQuery.value || mode.value || status.value))
const modeOptions = computed(() => [{ value: '', label: t('admin.accounts.authProfiles.allModes') }, { value: 'imap', label: 'IMAP · TLS' }, { value: 'api', label: t('admin.accounts.authProfiles.customCode') }])
const statusOptions = computed(() => [{ value: '', label: t('admin.accounts.authProfiles.allStatuses') }, { value: 'enabled', label: t('common.enabled') }, { value: 'disabled', label: t('common.disabled') }])
const columns = computed(() => [
  { key: 'name', label: t('admin.accounts.authProfiles.name') },
  { key: 'mailbox_address', label: t('admin.accounts.authProfiles.mailbox') },
  { key: 'mailbox_mode', label: t('admin.accounts.authProfiles.mode') },
  { key: 'secret', label: t('admin.accounts.authProfiles.secret') },
  { key: 'proxy_ids', label: t('admin.accounts.authProfiles.proxy') },
  { key: 'enabled', label: t('common.status') },
  { key: 'actions', label: t('common.actions') }
])
const filteredProfiles = computed(() => {
  const terms = searchQuery.value.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean)
  return profiles.value.filter(profile => {
    if (mode.value && profile.mailbox_mode !== mode.value) return false
    if (status.value && profile.enabled !== (status.value === 'enabled')) return false
    const text = [profile.name, profile.mailbox_address, profile.imap_host, profile.imap_username, endpoint(profile)].join(' ').toLocaleLowerCase()
    return terms.every(term => text.includes(term))
  }).sort((a, b) => a.name.localeCompare(b.name))
})
const totalPages = computed(() => Math.max(1, Math.ceil(filteredProfiles.value.length / pageSize.value)))
const page = computed({
  get: () => Math.min(totalPages.value, Math.max(1, Math.floor(Number(queryValue('page')) || 1))),
  set: value => { void router.replace({ query: { ...route.query, page: value > 1 ? String(value) : undefined } }) }
})
const pagedProfiles = computed(() => filteredProfiles.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
watch(pageSize, () => { page.value = 1 })
function endpoint(profile: ReauthProfile) {
  if (profile.mailbox_mode === 'imap') return `${profile.imap_host}:${profile.imap_port}`
  try { return new URL(profile.api_url).host } catch { return '' }
}
function proxyCount(profile: ReauthProfile) { return new Set([...(profile.proxy_ids || []), ...(profile.proxy_id ? [profile.proxy_id] : [])]).size }
function clearFilters() { void router.replace({ query: { ...route.query, q: undefined, mode: undefined, status: undefined, page: undefined } }) }
async function load() {
  credentialRevision.value++
  loading.value = true
  error.value = ''
  try { profiles.value = await listReauthProfiles() }
  catch (e) { error.value = extractApiErrorMessage(e, t('admin.accounts.authProfiles.failed')) }
  finally { loading.value = false }
}
function edit(profile: ReauthProfile | null) { current.value = profile; editing.value = true }
function askDelete(profile: ReauthProfile) { deleting.value = profile; deleteError.value = '' }
async function remove() {
  if (!deleting.value || removing.value) return
  removing.value = true
  deleteError.value = ''
  try { await deleteReauthProfile(deleting.value.id); deleting.value = null; await load() }
  catch (e) { deleteError.value = extractApiErrorMessage(e, t('admin.accounts.authProfiles.failed')) }
  finally { removing.value = false }
}
onMounted(load)
</script>
