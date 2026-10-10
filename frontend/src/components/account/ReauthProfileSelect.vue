<template>
  <Select
    :id="id"
    :model-value="modelValue"
    :options="options"
    :disabled="disabled"
    :searchable="true"
    :placeholder="t('admin.accounts.authProfiles.select')"
    :search-placeholder="t('admin.accounts.authProfiles.search')"
    :aria-label="t('admin.accounts.authProfiles.select')"
    @update:model-value="emit('update:modelValue', String($event ?? ''))"
  >
    <template #option="{ option, selected }">
      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2">
          <span class="truncate font-medium">{{ option.name || option.label }}</span>
          <span v-if="option.disabled && option.name" class="badge badge-gray">{{ t('common.disabled') }}</span>
        </div>
        <p v-if="option.mailbox" class="mt-0.5 truncate text-xs text-gray-500 dark:text-dark-400">{{ option.mailbox }}</p>
      </div>
      <Icon v-if="selected" name="check" size="sm" class="shrink-0 text-primary-500" />
    </template>
  </Select>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import type { ReauthProfile } from '@/api/admin/reauthProfiles'

const props = defineProps<{ modelValue: string; profiles: ReauthProfile[]; disabled?: boolean; id?: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const { t } = useI18n()
const options = computed<SelectOption[]>(() => {
  const items: SelectOption[] = [{ value: '', label: t('admin.accounts.authProfiles.select') }]
  if (props.modelValue && !props.profiles.some(profile => profile.id === props.modelValue)) {
    items.push({ value: props.modelValue, label: t('admin.accounts.authProfiles.missing'), disabled: true })
  }
  for (const profile of props.profiles) {
    const mailbox = profile.mailbox_address
    items.push({ value: profile.id, label: [profile.name, mailbox].filter(Boolean).join(' — '), name: profile.name, mailbox, disabled: !profile.enabled })
  }
  return items
})
</script>
