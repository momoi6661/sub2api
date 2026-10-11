import { apiClient } from '../client'
import type { Account } from '@/types'

export interface ReauthProfile {
  id: string
  name: string
  enabled: boolean
  mailbox_mode: 'imap' | 'api'
  api_result_type?: 'code' | 'auth_file'
  mailbox_address: string
  imap_host: string
  imap_port: number
  imap_username: string
  api_url: string
  code_script?: string
  has_code_script?: boolean
  folder: string
  sender_filter: string
  code_pattern: string
  mailbox_wait_seconds?: number
  email_resend_seconds?: number
  proxy_id: number | null
  proxy_ids?: number[]
  secret?: string
  has_secret: boolean
}

export async function listReauthProfiles(): Promise<ReauthProfile[]> {
  const { data } = await apiClient.get<ReauthProfile[]>('/admin/reauth-profiles')
  return data
}

export async function saveReauthProfile(profile: ReauthProfile): Promise<ReauthProfile> {
  const { data } = profile.id
    ? await apiClient.put<ReauthProfile>(`/admin/reauth-profiles/${profile.id}`, profile)
    : await apiClient.post<ReauthProfile>('/admin/reauth-profiles', profile)
  return data
}

export async function deleteReauthProfile(id: string): Promise<void> {
  await apiClient.delete(`/admin/reauth-profiles/${id}`)
}

export async function readReauthSecret(id: string, signal?: AbortSignal): Promise<string> {
  const { data } = await apiClient.post<{ secret: string }>(`/admin/reauth-profiles/${id}/secret`, {}, { signal })
  return data.secret
}

export interface ReauthScriptMaterial {
  code_script: string
  secret: string
  api_url: string
  mailbox_address: string
}
export async function readReauthScript(id: string): Promise<ReauthScriptMaterial> {
  const { data } = await apiClient.post<ReauthScriptMaterial>(`/admin/reauth-profiles/${id}/script-material`)
  return data
}

export async function applyReauthFile(accountID: number, content: string): Promise<Account> {
  const { data } = await apiClient.post<Account>(`/admin/accounts/${accountID}/apply-reauth-file`, { content })
  return data
}

export type AuthorizationStage = 'preparing' | 'configuration' | 'connecting' | 'protocol_authorization' | 'protocol_start' | 'token_exchange' | 'browser_authorization' | 'authorization_retry' | 'authorization_file' | 'verifying' | 'saving' | 'mailbox_connect' | 'browser_start' | 'email_login' | 'requesting_code' | 'waiting_code' | 'submitting_code' | 'consent'
export interface AuthorizationProgress { event: 'progress'; stage: AuthorizationStage }
export type ProgressCallback = (progress: AuthorizationProgress) => void

// Use the existing client/interceptors so streamed tasks retain login refresh,
// admin request headers, structured errors and cancellation behavior.
function authorizationRequestConfig(onProgress?: ProgressCallback, signal?: AbortSignal) {
  if (!onProgress) return { timeout: 615_000, signal }
  let offset = 0
  function consume(text: string) {
    while (true) {
      const newline = text.indexOf('\n', offset)
      if (newline < 0) break
      const line = text.slice(offset, newline).trim()
      offset = newline + 1
      if (!line) continue
      try {
        const event = JSON.parse(line)
        if (event.event === 'progress') onProgress?.(event)
      } catch { /* An interrupted frame is handled by the final response parser. */ }
    }
  }
  return {
    timeout: 615_000, signal, adapter: 'xhr' as const, responseType: 'text' as const,
    params: { stream: '1' },
    onDownloadProgress: (progress: { event?: ProgressEvent }) => {
      const request = progress.event?.target
      if (request instanceof XMLHttpRequest) consume(request.responseText)
    },
    transformResponse: [(text: string) => {
      consume(text)
      const last = text.trim().split('\n').pop()
      if (!last) throw new Error('Authorization stream ended without a result')
      const result = JSON.parse(last)
      if (typeof result.code !== 'number') throw new Error('Authorization stream ended without a result')
      return result
    }]
  }
}

export async function runOneClickAuth(accountID: number, onProgress?: ProgressCallback, signal?: AbortSignal): Promise<Account> {
  const { data } = await apiClient.post<Account>(`/admin/accounts/${accountID}/one-click-auth`, {}, authorizationRequestConfig(onProgress, signal))
  return data
}

export interface AuthorizationTask {
  id: number; account_id: number; account_name: string; login_email: string
  profile_id: string; profile_name: string; source: 'manual' | 'batch' | 'automatic' | 'new_account'
  status: 'queued' | 'running' | 'succeeded' | 'ready' | 'failed' | 'cancelled'; stage: AuthorizationStage
  created_at: string; started_at: string | null; finished_at: string | null
  events: { stage: AuthorizationStage; at: string }[]; error_code: string; error_message: string
}
export interface AuthorizationTaskPage { items: AuthorizationTask[]; total: number; active_count: number }
export async function submitAuthorizationTasks(accountIDs: number[], binding?: { profile_id: string; login_email: string }): Promise<AuthorizationTask[]> {
  const { data } = await apiClient.post<AuthorizationTask[]>('/admin/authorization-tasks', { account_ids: accountIDs, ...binding })
  return data
}
export async function listAuthorizationTasks(params: { q?: string; status?: string; page?: number; page_size?: number }, signal?: AbortSignal): Promise<AuthorizationTaskPage> {
  const { data } = await apiClient.get<AuthorizationTaskPage>('/admin/authorization-tasks', { params, signal })
  return data
}
export async function getAuthorizationTask(id: number, signal?: AbortSignal): Promise<AuthorizationTask> {
  const { data } = await apiClient.get<AuthorizationTask>(`/admin/authorization-tasks/${id}`, { signal })
  return data
}
export async function cancelAuthorizationTask(id: number): Promise<AuthorizationTask> {
  const { data } = await apiClient.post<AuthorizationTask>(`/admin/authorization-tasks/${id}/cancel`)
  return data
}
export async function deleteAuthorizationTask(id: number): Promise<void> {
  await apiClient.delete(`/admin/authorization-tasks/${id}`)
}

export interface NewAccountAuthorization {
  type: 'oauth'
  credentials: Record<string, unknown>
  extra?: Record<string, unknown>
}

export async function authorizeNewAccount(
  profileID: string, loginEmail: string, proxyID: number | null, signal: AbortSignal, onProgress?: ProgressCallback, workspaceIndex = 1
): Promise<NewAccountAuthorization> {
  const { data } = await apiClient.post<NewAccountAuthorization>('/admin/accounts/one-click-auth', {
    profile_id: profileID, login_email: loginEmail, proxy_id: proxyID, workspace_index: workspaceIndex
  }, authorizationRequestConfig(onProgress, signal))
  return data
}

export async function testReauthScript(profile: ReauthProfile, loginEmail: string, signal?: AbortSignal): Promise<void> {
  await apiClient.post('/admin/reauth-profiles/test-script', { profile, login_email: loginEmail }, { timeout: 40_000, signal })
}
