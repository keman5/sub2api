<template>
  <div ref="rootRef" class="flex flex-wrap items-center gap-3">
    <div class="w-full sm:w-64">
      <SearchSuggestInput
        :model-value="searchQuery"
        :placeholder="t('admin.accounts.searchAccounts')"
        :suggestions="accountSuggestions"
        :open="showDropdown"
        :empty-text="searchQuery ? t('common.noOptionsFound') : ''"
        @update:model-value="handleSearchModelUpdate"
        @search="handleSearch"
        @focus="onSearchFocus"
        @blur="onSearchBlur"
        @select="selectAccountOption"
        @clear="clearSearch"
      />
    </div>
    <Select :model-value="filters.platform" class="w-40" :options="pOpts" @update:model-value="updatePlatform" @change="$emit('change')" />
    <Select :model-value="filters.type" class="w-40" :options="tOpts" @update:model-value="updateType" @change="$emit('change')" />
    <Select :model-value="filters.status" class="w-40" :options="sOpts" @update:model-value="updateStatus" @change="$emit('change')" />
    <Select :model-value="filters.privacy_mode" class="w-40" :options="privacyOpts" @update:model-value="updatePrivacyMode" @change="$emit('change')" />
    <Select :model-value="filters.group" class="w-40" :options="gOpts" @update:model-value="updateGroup" @change="$emit('change')" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { adminAPI } from '@/api/admin'
import SearchSuggestInput, { type SearchSuggestOption } from '@/components/common/SearchSuggestInput.vue'
import Select from '@/components/common/Select.vue'
import type { AdminGroup } from '@/types'
import { CONCRETE_PLATFORM_OPTIONS } from '@/constants/platforms'
const props = defineProps<{ searchQuery: string; filters: Record<string, any>; groups?: AdminGroup[] }>()
const emit = defineEmits(['update:searchQuery', 'update:filters', 'change'])
const { t } = useI18n()

interface SimpleAccountSuggestion {
  id: number
  name: string
}

const rootRef = ref<HTMLElement | null>(null)
const accountSuggestions = ref<SearchSuggestOption<SimpleAccountSuggestion>[]>([])
const showDropdown = ref(false)
let accountSearchTimeout: ReturnType<typeof setTimeout> | null = null

const handleSearchModelUpdate = (value: string) => {
  emit('update:searchQuery', value)
}

const syncAccountSuggestions = (accounts: SimpleAccountSuggestion[]) => {
  const existingSuggestions = accountSuggestions.value
  const existingById = new Map(existingSuggestions.map((suggestion) => [suggestion.id, suggestion]))
  const nextSuggestions = accounts.map((account) => {
    const existing = existingById.get(account.id)
    const secondaryText = `#${account.id}`
    if (existing) {
      existing.primaryText = account.name
      existing.secondaryText = secondaryText
      existing.value = account
      return existing
    }
    return {
      id: account.id,
      primaryText: account.name,
      secondaryText,
      value: account
    }
  })

  const unchanged =
    existingSuggestions.length === nextSuggestions.length &&
    existingSuggestions.every((suggestion, index) => suggestion === nextSuggestions[index])

  if (!unchanged) {
    existingSuggestions.splice(0, existingSuggestions.length, ...nextSuggestions)
  }
}

const scheduleSuggestionLoad = (value: string, emitChangeAfterLoad: boolean) => {
  emit('update:searchQuery', value)
  showDropdown.value = true
  if (accountSearchTimeout) clearTimeout(accountSearchTimeout)
  accountSearchTimeout = setTimeout(async () => {
    try {
      const response = await adminAPI.accounts.list(1, 20, { search: value.trim() })
      syncAccountSuggestions(response.items.map((account) => ({
        id: account.id,
        name: account.name
      })))
    } catch {
      syncAccountSuggestions([])
    }
    if (emitChangeAfterLoad) {
      emit('change')
    }
  }, 300)
}

const handleSearch = (value: string) => {
  scheduleSuggestionLoad(value, true)
}

const onSearchFocus = () => {
  showDropdown.value = true
  if (accountSuggestions.value.length > 0) {
    return
  }
  scheduleSuggestionLoad(props.searchQuery, false)
}

const onSearchBlur = () => {
  showDropdown.value = false
}

const selectAccountOption = (option: SearchSuggestOption<SimpleAccountSuggestion>) => {
  emit('update:searchQuery', option.primaryText)
  showDropdown.value = false
  emit('change')
}

const clearSearch = () => {
  emit('update:searchQuery', '')
  syncAccountSuggestions([])
  showDropdown.value = false
  emit('change')
}

const onDocumentClick = (event: MouseEvent) => {
  const target = event.target as Node | null
  if (!target) return
  if (!(rootRef.value?.contains(target) ?? false)) {
    showDropdown.value = false
  }
}

const updatePlatform = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, platform: value }) }
const updateType = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, type: value }) }
const updateStatus = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, status: value }) }
const updatePrivacyMode = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, privacy_mode: value }) }
const updateGroup = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, group: value }) }
const pOpts = computed(() => [{ value: '', label: t('admin.accounts.allPlatforms') }, ...CONCRETE_PLATFORM_OPTIONS])
const tOpts = computed(() => [{ value: '', label: t('admin.accounts.allTypes') }, { value: 'oauth', label: t('admin.accounts.oauthType') }, { value: 'setup-token', label: t('admin.accounts.setupToken') }, { value: 'apikey', label: t('admin.accounts.apiKey') }, { value: 'bedrock', label: 'AWS Bedrock' }])
const sOpts = computed(() => [{ value: '', label: t('admin.accounts.allStatus') }, { value: 'active', label: t('admin.accounts.status.active') }, { value: 'inactive', label: t('admin.accounts.status.inactive') }, { value: 'error', label: t('admin.accounts.status.error') }, { value: 'rate_limited', label: t('admin.accounts.status.rateLimited') }, { value: 'temp_unschedulable', label: t('admin.accounts.status.tempUnschedulable') }, { value: 'unschedulable', label: t('admin.accounts.status.unschedulable') }])
const privacyOpts = computed(() => [
  { value: '', label: t('admin.accounts.allPrivacyModes') },
  { value: '__unset__', label: t('admin.accounts.privacyUnset') },
  { value: 'training_off', label: 'Privacy' },
  { value: 'training_set_cf_blocked', label: 'CF' },
  { value: 'training_set_failed', label: 'Fail' }
])
const gOpts = computed(() => [
  { value: '', label: t('admin.accounts.allGroups') },
  { value: 'ungrouped', label: t('admin.accounts.ungroupedGroup') },
  ...(props.groups || []).map(g => ({ value: String(g.id), label: g.name }))
])

onMounted(() => {
  document.addEventListener('click', onDocumentClick)
})

onUnmounted(() => {
  document.removeEventListener('click', onDocumentClick)
  if (accountSearchTimeout) {
    clearTimeout(accountSearchTimeout)
  }
})
</script>
