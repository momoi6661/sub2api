<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-3"><h2 class="text-lg font-semibold">{{ t('admin.accounts.authProfiles.title') }}</h2><div class="flex gap-2"><button class="btn btn-secondary" :disabled="loading" @click="load">{{ t('common.refresh') }}</button><button class="btn btn-primary" @click="edit(null)">{{ t('admin.accounts.authProfiles.create') }}</button></div></div>
      <p class="text-sm text-gray-500">{{ t('admin.accounts.authProfiles.description') }}</p>
      <p class="text-sm text-amber-600">{{ t('admin.accounts.authProfiles.pending') }}</p>
      <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
      <p v-if="loading" role="status">{{ t('common.loading') }}</p>
      <p v-else-if="!profiles.length && !error" class="py-12 text-center text-gray-500">{{ t('admin.accounts.authProfiles.empty') }}</p>
      <div v-else class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-600">
        <table class="w-full text-left text-sm"><thead class="bg-gray-50 dark:bg-dark-700"><tr><th class="p-3">{{ t('admin.accounts.authProfiles.name') }}</th><th class="p-3">{{ t('admin.accounts.authProfiles.mailbox') }}</th><th class="p-3">{{ t('admin.accounts.authProfiles.mode') }}</th><th class="p-3">{{ t('admin.accounts.authProfiles.enabled') }}</th><th class="p-3">{{ t('common.actions') }}</th></tr></thead>
          <tbody><tr v-for="profile in profiles" :key="profile.id" class="border-t border-gray-200 dark:border-dark-600"><td class="p-3">{{ profile.name }}</td><td class="p-3">{{ profile.mailbox_address }}</td><td class="p-3">{{ profile.mailbox_mode.toUpperCase() }}</td><td class="p-3">{{ t(profile.enabled ? 'common.yes' : 'common.no') }}</td><td class="flex gap-2 p-3"><button class="btn btn-secondary" @click="edit(profile)">{{ t('common.edit') }}</button><button class="btn btn-danger" @click="deleting = profile">{{ t('common.delete') }}</button></td></tr></tbody>
        </table>
      </div>
    </div>
    <ReauthProfileEditor :show="editing" :profile="current" @close="editing = false" @saved="load" />
    <BaseDialog :show="!!deleting" :title="t('common.delete')" @close="deleting = null"><p>{{ t('admin.accounts.authProfiles.deleteConfirm', { name: deleting?.name }) }}</p><p v-if="deleteError" class="mt-3 text-red-600" role="alert">{{ deleteError }}</p><template #footer><button class="btn btn-secondary" :disabled="removing" @click="deleting = null">{{ t('common.cancel') }}</button><button class="btn btn-danger" :disabled="removing" @click="remove">{{ t('common.delete') }}</button></template></BaseDialog>
  </AppLayout>
</template>
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ReauthProfileEditor from '@/components/account/ReauthProfileEditor.vue'
import { listReauthProfiles, deleteReauthProfile, type ReauthProfile } from '@/api/admin/reauthProfiles'
const { t } = useI18n()
const profiles = ref<ReauthProfile[]>([])
const loading = ref(false), editing = ref(false), removing = ref(false)
const current = ref<ReauthProfile | null>(null), deleting = ref<ReauthProfile | null>(null)
const error = ref(''), deleteError = ref('')
async function load() { loading.value = true; error.value = ''; try { profiles.value = await listReauthProfiles() } catch (e: any) { error.value = e?.message || t('admin.accounts.authProfiles.failed') } finally { loading.value = false } }
function edit(profile: ReauthProfile | null) { current.value = profile; editing.value = true }
async function remove() { if (!deleting.value || removing.value) return; removing.value = true; deleteError.value = ''; try { await deleteReauthProfile(deleting.value.id); deleting.value = null; await load() } catch (e: any) { deleteError.value = e?.message || t('admin.accounts.authProfiles.failed') } finally { removing.value = false } }
onMounted(load)
</script>
