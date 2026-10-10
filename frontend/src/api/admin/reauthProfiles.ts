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

export async function runOneClickAuth(accountID: number): Promise<Account> {
  const { data } = await apiClient.post<Account>(`/admin/accounts/${accountID}/one-click-auth`, {}, { timeout: 145_000 })
  return data
}

export interface NewAccountAuthorization {
  type: 'oauth'
  credentials: Record<string, unknown>
  extra?: Record<string, unknown>
}

export async function authorizeNewAccount(
  profileID: string, loginEmail: string, proxyID: number | null, signal: AbortSignal
): Promise<NewAccountAuthorization> {
  const { data } = await apiClient.post<NewAccountAuthorization>('/admin/accounts/one-click-auth', {
    profile_id: profileID, login_email: loginEmail, proxy_id: proxyID
  }, { timeout: 145_000, signal })
  return data
}

export async function testReauthScript(profile: ReauthProfile, loginEmail: string, signal?: AbortSignal): Promise<void> {
  await apiClient.post('/admin/reauth-profiles/test-script', { profile, login_email: loginEmail }, { timeout: 40_000, signal })
}
