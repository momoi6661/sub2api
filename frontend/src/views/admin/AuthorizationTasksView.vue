<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div class="min-w-0">
            <h2 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t(`${key}.title`) }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t(`${key}.description`) }}</p>
          </div>
          <div class="flex items-center gap-2">
            <RouterLink to="/admin/reauth-profiles" class="btn btn-secondary">{{ t('admin.accounts.authProfiles.title') }}</RouterLink>
            <button class="btn btn-secondary" :disabled="loading" :aria-label="t('common.refresh')" :title="t('common.refresh')" @click="load"><Icon name="refresh" size="md" :class="loading && 'animate-spin'" /></button>
          </div>
        </div>
      </template>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="w-full sm:w-96"><SearchInput v-model="search" :placeholder="t(`${key}.search`)" :aria-label="t(`${key}.search`)" /></div>
          <div class="w-full sm:w-44"><Select v-model="status" :options="statusOptions" :aria-label="t('common.status')" /></div>
          <button v-if="search || status" class="btn btn-ghost btn-sm" @click="clearFilters">{{ t('admin.accounts.authProfiles.clearFilters') }}</button>
          <span v-if="activeCount" class="badge badge-info sm:ml-auto">{{ t('admin.accounts.oneClickAuth.runningCount', { count: activeCount }) }}</span>
          <span class="text-sm tabular-nums text-gray-500 dark:text-dark-400" :class="!activeCount && 'sm:ml-auto'">{{ t(`${key}.count`, { count: total }) }}</span>
        </div>
        <p v-if="error" role="alert" class="mt-3 rounded-lg bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-400">{{ error }}</p>
      </template>
      <template #table>
        <DataTable :columns="columns" :data="tasks" :loading="loading && !tasks.length" row-key="id" :expandable-actions="false">
          <template #cell-account_name="{ row }">
            <button class="max-w-72 text-left text-sm font-medium text-primary-600 hover:underline dark:text-primary-400" @click="open(row)"><span class="block truncate">{{ row.account_name || row.login_email || t(`${key}.newAccount`) }}</span></button>
            <p class="mt-1 max-w-72 truncate text-xs text-gray-500 dark:text-dark-400" :title="row.login_email">{{ row.login_email }} · #{{ row.id }}</p>
          </template>
          <template #cell-profile_name="{ row }"><span class="block max-w-52 truncate" :title="row.profile_name || row.profile_id">{{ row.profile_name || row.profile_id || '—' }}</span><p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t(`${key}.sources.${row.source}`) }}</p></template>
          <template #cell-status="{ row }"><span :class="['badge', badge(row.status)]">{{ t(`${key}.statuses.${row.status}`) }}</span><p class="mt-1 max-w-64 text-xs text-gray-500 dark:text-dark-400">{{ row.status === 'queued' ? t(`${key}.queueHint`) : row.error_code || t(`admin.accounts.oneClickAuth.${row.stage}`) }}</p></template>
          <template #cell-created_at="{ row }"><span class="whitespace-nowrap text-sm tabular-nums">{{ date(row.created_at) }}</span><p class="mt-1 text-xs tabular-nums text-gray-500 dark:text-dark-400">{{ row.started_at ? t('admin.accounts.oneClickAuth.elapsed', { seconds: duration(row) }) : '—' }}</p></template>
          <template #cell-actions="{ row }"><button class="btn btn-secondary btn-sm" @click="open(row)">{{ t(`${key}.details`) }}</button></template>
          <template #empty>
            <div class="px-6 py-14 text-center"><Icon name="clock" size="xl" class="mx-auto mb-3 text-gray-400" /><p class="font-medium text-gray-900 dark:text-white">{{ t(`${key}.${search || status ? 'noMatches' : 'empty'}`) }}</p><p class="mt-2 text-sm text-gray-500 dark:text-dark-400">{{ t(`${key}.${search || status ? 'noMatchesHint' : 'emptyHint'}`) }}</p></div>
          </template>
        </DataTable>
      </template>
      <template #pagination><Pagination :page="page" :page-size="20" :total="total" :show-page-size-selector="false" @update:page="setPage" /></template>
    </TablePageLayout>
    <BaseDialog :show="!!selected" :title="t(`${key}.details`)" @close="close">
      <div v-if="selected" class="space-y-5">
        <dl class="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">
          <div><dt class="text-gray-500 dark:text-dark-400">{{ t(`${key}.taskId`) }}</dt><dd class="mt-1">#{{ selected.id }} · {{ t(`${key}.sources.${selected.source}`) }}</dd></div>
          <div><dt class="text-gray-500 dark:text-dark-400">{{ t('admin.accounts.oneClickAuth.profile') }}</dt><dd class="mt-1 break-words">{{ selected.profile_name || selected.profile_id || '—' }}</dd></div>
          <div class="sm:col-span-2"><dt class="text-gray-500 dark:text-dark-400">{{ t('admin.accounts.authProfiles.loginEmail') }}</dt><dd class="mt-1 break-all">{{ selected.login_email || '—' }}</dd></div>
        </dl>
        <p v-if="selected.status === 'queued'" role="status" class="rounded-lg bg-gray-50 p-4 text-sm dark:bg-dark-700">{{ t(`${key}.queueHint`) }}</p>
        <OneClickAuthProgress v-else :stage="selected.stage" :status="selected.status" :started-at="Date.parse(selected.started_at || selected.created_at)" :finished-at="selected.finished_at ? Date.parse(selected.finished_at) : undefined" :error="selected.error_code ? errorHint(selected) : undefined" />
        <div>
          <h3 class="mb-3 text-sm font-medium">{{ t(`${key}.timeline`) }}</h3>
          <ol class="space-y-3 border-l border-gray-200 pl-4 text-sm dark:border-dark-600">
            <li v-for="(event, index) in selected.events" :key="index"><time class="text-xs tabular-nums text-gray-500 dark:text-dark-400">{{ date(event.at) }}</time><p class="mt-0.5">{{ t(`admin.accounts.oneClickAuth.${event.stage}`) }}</p></li>
            <li v-if="selected.finished_at"><time class="text-xs tabular-nums text-gray-500 dark:text-dark-400">{{ date(selected.finished_at) }}</time><p class="mt-0.5">{{ t(`${key}.statuses.${selected.status}`) }}</p></li>
          </ol>
        </div>
        <p v-if="detailError" role="alert" class="text-sm text-red-600">{{ detailError }}</p>
      </div>
      <template #footer>
        <button v-if="selected && (selected.status === 'queued' || selected.status === 'running')" class="btn btn-danger" :disabled="stopping" @click="stop">{{ t(`${key}.cancel`) }}</button>
        <button v-if="selected?.status === 'failed' && selected.account_id" class="btn btn-primary" :disabled="retrying || stopping" @click="retry">{{ t(`${key}.retry`) }}</button>
        <button v-if="selected && !['queued', 'running'].includes(selected.status)" class="btn btn-ghost text-red-600" :disabled="stopping" @click="remove">{{ t(`${key}.delete`) }}</button>
        <button class="btn btn-secondary" @click="close">{{ t('common.close') }}</button>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue'
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
import OneClickAuthProgress from '@/components/account/OneClickAuthProgress.vue'
import { listAuthorizationTasks, getAuthorizationTask, submitAuthorizationTasks, cancelAuthorizationTask, deleteAuthorizationTask, type AuthorizationTask } from '@/api/admin/reauthProfiles'
import { extractApiErrorMessage } from '@/utils/apiError'

const key = 'admin.accounts.authorizationTasks'
const { t, te } = useI18n()
const route = useRoute(), router = useRouter()
const tasks = ref<AuthorizationTask[]>([]), selected = ref<AuthorizationTask | null>(null)
const total = ref(0), activeCount = ref(0), loading = ref(false), retrying = ref(false), stopping = ref(false)
const error = ref(''), detailError = ref(''), now = ref(Date.now())
let refreshTimer: ReturnType<typeof setTimeout> | undefined, searchTimer: ReturnType<typeof setTimeout> | undefined
let request: AbortController | undefined, disposed = false
const query = (name: string) => typeof route.query[name] === 'string' ? route.query[name] as string : ''
const search = computed({ get: () => query('q'), set: value => { void router.replace({ query: { ...route.query, q: value || undefined, page: undefined } }) } })
const status = computed({ get: () => query('status'), set: value => { void router.replace({ query: { ...route.query, status: value || undefined, page: undefined } }) } })
const page = computed(() => Math.max(1, Math.min(1000000, Number(query('page')) || 1)))
const statusOptions = computed(() => [{ value: '', label: t('admin.accounts.authProfiles.allStatuses') }, ...['queued', 'running', 'succeeded', 'ready', 'failed', 'cancelled'].map(value => ({ value, label: t(`${key}.statuses.${value}`) }))])
const columns = computed(() => [
  { key: 'account_name', label: t(`${key}.account`) }, { key: 'profile_name', label: t('admin.accounts.oneClickAuth.profile') },
  { key: 'status', label: t('common.status') }, { key: 'created_at', label: t(`${key}.createdAt`) }, { key: 'actions', label: t('common.actions') }
])
function badge(value: string) { return value === 'succeeded' || value === 'ready' ? 'badge-success' : value === 'failed' ? 'badge-danger' : value === 'cancelled' ? 'badge-warning' : value === 'running' ? 'badge-info' : 'badge-gray' }
function date(value: string) { return new Date(value).toLocaleString() }
function duration(task: AuthorizationTask) { return Math.max(0, Math.floor(((task.finished_at ? Date.parse(task.finished_at) : now.value) - Date.parse(task.started_at || task.created_at)) / 1000)) }
function errorHint(task: AuthorizationTask) {
  const code = task.error_code
  const status = ['AUTH_PAGE_HTTP_ERROR', 'MANUAL_REQUIRED'].includes(code) ? /^Authorization page returned HTTP ([45]\d{2})$/.exec(task.error_message || '')?.[1] : undefined
  return `${code}${status ? ` (HTTP ${status})` : ''} · ${te(`${key}.errors.${code}`) ? t(`${key}.errors.${code}`) : t(`${key}.errorHint`)}`
}
function setPage(value: number) { void router.replace({ query: { ...route.query, page: value > 1 ? String(value) : undefined } }) }
function clearFilters() { void router.replace({ query: { task: route.query.task } }) }
function close() { selected.value = null; detailError.value = ''; void router.replace({ query: { ...route.query, task: undefined } }) }
function open(task: AuthorizationTask) { selected.value = task; detailError.value = ''; void router.replace({ query: { ...route.query, task: String(task.id) } }) }
function scheduleRefresh() {
  clearTimeout(refreshTimer)
  if (!disposed && !document.hidden && (activeCount.value || selected.value?.status === 'running' || selected.value?.status === 'queued')) refreshTimer = setTimeout(() => { void load() }, 4000)
}
async function load() {
  clearTimeout(refreshTimer)
  request?.abort()
  const controller = new AbortController(); request = controller
  loading.value = true; error.value = ''
  try {
    const data = await listAuthorizationTasks({ q: search.value, status: status.value, page: page.value, page_size: 20 }, controller.signal)
    if (controller.signal.aborted || disposed) return
    tasks.value = data.items; total.value = data.total; activeCount.value = data.active_count; now.value = Date.now()
    const id = Number(query('task'))
    if (id > 0) {
      const task = data.items.find(item => item.id === id) || await getAuthorizationTask(id, controller.signal)
      if (!controller.signal.aborted && !disposed && Number(query('task')) === id) selected.value = task
    }
  } catch (e) { if (!controller.signal.aborted) error.value = extractApiErrorMessage(e, t(`${key}.loadFailed`)) }
  finally { if (request === controller) { loading.value = false; if (!error.value) scheduleRefresh() } }
}
async function retry() {
  if (!selected.value?.account_id || retrying.value) return
  retrying.value = true; detailError.value = ''
  try { const [task] = await submitAuthorizationTasks([selected.value.account_id]); open(task); await load() }
  catch (e) { detailError.value = extractApiErrorMessage(e, t(`${key}.loadFailed`)) }
  finally { retrying.value = false }
}
async function stop() {
  if (!selected.value || stopping.value) return
  stopping.value = true; detailError.value = ''
  try { selected.value = await cancelAuthorizationTask(selected.value.id); await load() }
  catch (e) { detailError.value = extractApiErrorMessage(e, t(`${key}.loadFailed`)) }
  finally { stopping.value = false }
}
async function remove() {
  if (!selected.value || stopping.value) return
  stopping.value = true; detailError.value = ''
  try { await deleteAuthorizationTask(selected.value.id); close(); await load() }
  catch (e) { detailError.value = extractApiErrorMessage(e, t(`${key}.loadFailed`)) }
  finally { stopping.value = false }
}
function visibility() { if (document.hidden) { clearTimeout(refreshTimer); request?.abort() } else void load() }
watch(() => [route.query.q, route.query.status, route.query.page], () => { clearTimeout(refreshTimer); clearTimeout(searchTimer); request?.abort(); searchTimer = setTimeout(() => { void load() }, 250) })
watch(() => route.query.task, value => { if (!value) selected.value = null; else if (selected.value?.id !== Number(value)) void load(); scheduleRefresh() })
onMounted(() => { document.addEventListener('visibilitychange', visibility); void load() })
onBeforeUnmount(() => { disposed = true; request?.abort(); clearTimeout(refreshTimer); clearTimeout(searchTimer); document.removeEventListener('visibilitychange', visibility) })
</script>
