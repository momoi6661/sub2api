<template>
  <details class="rounded-lg border border-gray-200 p-3 text-sm dark:border-dark-600" open>
    <summary class="cursor-pointer font-medium">{{ t('admin.accounts.authProfiles.rulesTitle') }}</summary>
    <div class="mt-3 space-y-3">
      <p>{{ t('admin.accounts.authProfiles.rulesContext') }}</p>
      <p>{{ t('admin.accounts.authProfiles.rulesReturn') }}</p>
      <pre class="overflow-x-auto rounded-md bg-gray-50 p-3 text-xs dark:bg-dark-700">{{ examples }}</pre>
      <p>{{ t('admin.accounts.authProfiles.rulesErrors') }}</p>
      <p>{{ t('admin.accounts.authProfiles.rulesTimeout') }}</p>
    </div>
  </details>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
const examples = `// 1. code
return { type: 'code', code: '123456' };

// 2. auth_file
return { type: 'auth_file', auth: authJson };

// 3. error
return {
  type: 'error',
  error: {
    code: 'MAILBOX_TIMEOUT',
    message: 'No verification code received',
    retryable: true
  }
};`
</script>
