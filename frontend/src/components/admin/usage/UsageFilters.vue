<template>
  <div :class="flat ? 'p-4 sm:p-6' : 'card p-6'">
    <!-- Toolbar: left filters (multi-line) + right actions -->
    <div class="flex flex-wrap items-end justify-between gap-4">
      <!-- Left: filters (allowed to wrap to multiple rows) -->
      <div class="flex flex-1 flex-wrap items-end gap-4">
        <!-- User Search -->
        <div ref="userSearchRef" class="usage-filter-dropdown relative w-full sm:w-auto sm:min-w-[240px]">
          <label class="input-label">{{ t('admin.usage.userFilter') }}</label>
          <SearchSuggestInput
            v-model="userKeyword"
            :placeholder="t('admin.usage.searchUserPlaceholder')"
            :suggestions="userSuggestions"
            :open="showUserDropdown"
            :empty-text="userKeyword ? t('common.noOptionsFound') : ''"
            @search="handleUserSearch"
            @focus="onUserFocus"
            @blur="onUserBlur"
            @select="selectUserOption"
            @clear="clearUser"
          />
        </div>

        <!-- API Key Search -->
        <div ref="apiKeySearchRef" class="usage-filter-dropdown relative w-full sm:w-auto sm:min-w-[240px]">
          <label class="input-label">{{ t('usage.apiKeyFilter') }}</label>
          <SearchSuggestInput
            v-model="apiKeyKeyword"
            :placeholder="t('admin.usage.searchApiKeyPlaceholder')"
            :suggestions="apiKeySuggestions"
            :open="showApiKeyDropdown"
            @search="handleApiKeySearch"
            @focus="onApiKeyFocus"
            @blur="onApiKeyBlur"
            @select="selectApiKeyOption"
            @clear="onClearApiKey"
          />
        </div>

        <!-- Model Filter -->
        <div class="w-full sm:w-auto sm:min-w-[220px]">
          <label class="input-label">{{ t('usage.model') }}</label>
          <Select v-model="filters.model" :options="modelOptions" searchable @change="emitChange" />
        </div>

        <!-- Account Filter -->
        <div ref="accountSearchRef" class="usage-filter-dropdown relative w-full sm:w-auto sm:min-w-[220px]">
          <label class="input-label">{{ t('admin.usage.account') }}</label>
          <SearchSuggestInput
            v-model="accountKeyword"
            :placeholder="t('admin.usage.searchAccountPlaceholder')"
            :suggestions="accountSuggestions"
            :open="showAccountDropdown"
            :empty-text="accountKeyword ? t('common.noOptionsFound') : ''"
            @search="handleAccountSearch"
            @focus="onAccountFocus"
            @blur="onAccountBlur"
            @select="selectAccountOption"
            @clear="clearAccount"
          />
        </div>

        <!-- Request Type Filter (usage only) -->
        <div v-if="mode !== 'errors'" class="w-full sm:w-auto sm:min-w-[180px]">
          <label class="input-label">{{ t('usage.type') }}</label>
          <Select v-model="filters.request_type" :options="requestTypeOptions" @change="emitChange" />
        </div>

        <!-- Native compaction is independent of the transport request type. -->
        <div v-if="mode !== 'errors'" class="w-full sm:w-auto sm:min-w-[180px]">
          <label class="input-label">{{ t('usage.compactionFilter') }}</label>
          <Select v-model="filters.native_compaction_v2" :options="compactionOptions" @change="emitChange" />
        </div>

        <!-- Billing Type Filter (usage only) -->
        <div v-if="mode !== 'errors'" class="w-full sm:w-auto sm:min-w-[200px]">
          <label class="input-label">{{ t('admin.usage.billingType') }}</label>
          <Select v-model="filters.billing_type" :options="billingTypeOptions" @change="emitChange" />
        </div>

        <!-- Billing Mode Filter (usage only；用户排行的 user-breakdown 接口不支持该维度) -->
        <div v-if="mode === 'usage'" class="w-full sm:w-auto sm:min-w-[200px]">
          <label class="input-label">{{ t('admin.usage.billingMode') }}</label>
          <Select v-model="filters.billing_mode" :options="billingModeOptions" @change="emitChange" />
        </div>

        <div v-if="mode === 'usage'" class="w-full sm:w-auto sm:min-w-[220px]">
          <label class="input-label">{{ t('admin.usage.upstreamModelAudit') }}</label>
          <Select v-model="filters.upstream_model_mismatch" :options="upstreamModelMismatchOptions" @change="emitChange" />
        </div>

        <!-- Error Phase Filter (errors only) -->
        <div v-if="mode === 'errors'" class="w-full sm:w-auto sm:min-w-[180px]">
          <label class="input-label">{{ t('admin.ops.errorLog.type') }}</label>
          <Select v-model="filters.error_phase" :options="errorPhaseOptions" @change="emitChange" />
        </div>

        <!-- Error Category Filter (errors only) -->
        <div v-if="mode === 'errors'" class="w-full sm:w-auto sm:min-w-[180px]">
          <label class="input-label">{{ t('usage.errors.category') }}</label>
          <Select v-model="filters.error_category" :options="errorCategoryOptions" @change="emitChange" />
        </div>

        <!-- Status Code Filter (errors only) -->
        <div v-if="mode === 'errors'" class="w-full sm:w-auto sm:min-w-[180px]">
          <label class="input-label">{{ t('admin.ops.errorLog.status') }}</label>
          <Select v-model="filters.status_code" :options="statusCodeOptions" @change="emitChange" />
        </div>

        <!-- Group Filter -->
        <div class="w-full sm:w-auto sm:min-w-[200px]">
          <label class="input-label">{{ t('admin.usage.group') }}</label>
          <Select v-model="filters.group_id" :options="groupOptions" searchable @change="emitChange" />
        </div>

      </div>

      <!-- Right: actions -->
      <div v-if="showActions" class="flex w-full flex-wrap items-center justify-end gap-3 sm:w-auto">
        <button type="button" @click="$emit('refresh')" class="btn btn-secondary">
          {{ t('common.refresh') }}
        </button>
        <button type="button" @click="$emit('reset')" class="btn btn-secondary">
          {{ t('common.reset') }}
        </button>
        <slot name="after-reset" />
        <template v-if="mode === 'usage'">
          <button type="button" @click="$emit('cleanup')" class="btn btn-danger">
            {{ t('admin.usage.cleanup.button') }}
          </button>
          <button type="button" @click="$emit('export')" :disabled="exporting" class="btn btn-primary">
            {{ t('usage.exportExcel') }}
          </button>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, toRef, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import SearchSuggestInput, { type SearchSuggestOption } from '@/components/common/SearchSuggestInput.vue'
import { COMMON_ERROR_STATUS_CODES } from '@/utils/errorBadges'
import type { SimpleApiKey, SimpleUser } from '@/api/admin/usage'

type ModelValue = Record<string, any>

interface Props {
  modelValue: ModelValue
  exporting: boolean
  startDate: string
  endDate: string
  showActions?: boolean
  modelOptions?: string[]
  /**
   * errors 模式:隐藏用量专属字段/按钮,显示错误类型+状态码(错误请求 tab 用)
   * ranking 模式:同 usage 但隐藏计费模式筛选与清理/导出按钮(用户排行 tab 用)
   */
  mode?: 'usage' | 'errors' | 'ranking'
  /** 嵌入统一卡片内使用：去掉自身卡片外观 */
  flat?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  showActions: true,
  mode: 'usage',
  flat: false
})
const emit = defineEmits([
  'update:modelValue',
  'change',
  'refresh',
  'reset',
  'export',
  'cleanup'
])

const { t } = useI18n()
const filters = toRef(props, 'modelValue')

const userSearchRef = ref<HTMLElement | null>(null)
const apiKeySearchRef = ref<HTMLElement | null>(null)
const accountSearchRef = ref<HTMLElement | null>(null)

const userKeyword = computed({
  get: () => String(filters.value.user_search ?? ''),
  set: (value: string) => {
    filters.value.user_search = value
  }
})
const userSuggestions = ref<SearchSuggestOption<SimpleUser>[]>([])
const showUserDropdown = ref(false)
let userSearchTimeout: ReturnType<typeof setTimeout> | null = null
let userSearchSequence = 0

const apiKeyKeyword = computed({
  get: () => String(filters.value.api_key_search ?? ''),
  set: (value: string) => {
    filters.value.api_key_search = value
  }
})
const apiKeySuggestions = ref<SearchSuggestOption<SimpleApiKey>[]>([])
const showApiKeyDropdown = ref(false)
let apiKeySearchTimeout: ReturnType<typeof setTimeout> | null = null

interface SimpleAccount {
  id: number
  name: string
}
const accountKeyword = computed({
  get: () => String(filters.value.account_search ?? ''),
  set: (value: string) => {
    filters.value.account_search = value
  }
})
const accountSuggestions = ref<SearchSuggestOption<SimpleAccount>[]>([])
const showAccountDropdown = ref(false)
let accountSearchTimeout: ReturnType<typeof setTimeout> | null = null

const syncSuggestionItems = <T extends { id: string | number }>(
  target: typeof userSuggestions | typeof apiKeySuggestions | typeof accountSuggestions,
  items: T[],
  buildTexts: (item: T) => { primaryText: string; secondaryText?: string }
) => {
  const existingSuggestions = target.value as SearchSuggestOption<T>[]
  const existingById = new Map(existingSuggestions.map((suggestion) => [suggestion.id, suggestion]))
  const nextSuggestions = items.map((item) => {
    const existing = existingById.get(item.id)
    const texts = buildTexts(item)
    if (existing) {
      existing.primaryText = texts.primaryText
      existing.secondaryText = texts.secondaryText
      existing.value = item
      return existing
    }
    return {
      id: item.id,
      primaryText: texts.primaryText,
      secondaryText: texts.secondaryText,
      value: item,
    }
  })

  const unchanged =
    existingSuggestions.length === nextSuggestions.length &&
    existingSuggestions.every((suggestion, index) => suggestion === nextSuggestions[index])

  if (!unchanged) {
    existingSuggestions.splice(0, existingSuggestions.length, ...nextSuggestions)
  }
}

const modelOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.usage.allModels') },
  ...(props.modelOptions ?? []).map((m) => ({ value: m, label: m })),
])
const groupOptions = ref<SelectOption[]>([{ value: null, label: t('admin.usage.allGroups') }])

const requestTypeOptions = ref<SelectOption[]>([
  { value: null, label: t('admin.usage.allTypes') },
  { value: 'ws_v2', label: t('usage.ws') },
  { value: 'live', label: t('usage.live') },
  { value: 'stream', label: t('usage.stream') },
  { value: 'sync', label: t('usage.sync') },
  { value: 'cyber', label: t('usage.cyber') }
])

const compactionOptions = ref<SelectOption[]>([
  { value: null, label: t('usage.allCompactionTypes') },
  { value: true, label: t('usage.compactionOnly') }
])

const billingTypeOptions = ref<SelectOption[]>([
  { value: null, label: t('admin.usage.allBillingTypes') },
  { value: 0, label: t('admin.usage.billingTypeBalance') },
  { value: 1, label: t('admin.usage.billingTypeSubscription') }
])

// 错误类型对应后端 phase 参数(与错误表"类型"徽章同语义)
const errorPhaseOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('admin.usage.allTypes') },
  { value: 'upstream', label: t('admin.ops.errorLog.typeUpstream') },
  { value: 'account_auth', label: t('admin.ops.errorLog.typeAccountAuth') },
  { value: 'request', label: t('admin.ops.errorLog.typeRequest') },
  { value: 'auth', label: t('admin.ops.errorLog.typeAuth') },
  { value: 'routing', label: t('admin.ops.errorLog.typeRouting') },
  { value: 'internal', label: t('admin.ops.errorLog.typeInternal') },
])

// 分类码同用户端 /usage 错误筛选;"other" 无法反查为过滤条件,刻意不列
const errorCategoryCodes = ['auth', 'rate_limit', 'quota', 'invalid_request', 'service_unavailable', 'upstream', 'internal', 'cyber']

const errorCategoryOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('usage.errors.allCategories') },
  ...errorCategoryCodes.map((c) => ({ value: c, label: t('usage.errors.categories.' + c) })),
])

const statusCodeOptions = computed<SelectOption[]>(() => [
  { value: null, label: t('usage.errors.allStatuses') },
  ...COMMON_ERROR_STATUS_CODES.map((c) => ({ value: c, label: String(c) })),
])

const billingModeOptions = ref<SelectOption[]>([
  { value: null, label: t('admin.usage.allBillingModes') },
  { value: 'token', label: t('admin.usage.billingModeToken') },
  { value: 'per_request', label: t('admin.usage.billingModePerRequest') },
  { value: 'image', label: t('admin.usage.billingModeImage') },
  { value: 'video', label: t('admin.usage.billingModeVideo') }
])

const upstreamModelMismatchOptions = ref<SelectOption[]>([
  { value: null, label: t('admin.usage.allUpstreamModelAudit') },
  { value: true, label: t('admin.usage.upstreamModelMismatchOnly') },
  { value: false, label: t('admin.usage.upstreamModelMatchedOnly') }
])

const emitChange = () => emit('change')

const clearPendingUserSearch = () => {
  if (userSearchTimeout) {
    clearTimeout(userSearchTimeout)
    userSearchTimeout = null
  }
  userSearchSequence += 1
}

const debounceUserSearch = () => {
  clearPendingUserSearch()
  const query = userKeyword.value.trim()
  const sequence = userSearchSequence
  userSearchTimeout = setTimeout(async () => {
    userSearchTimeout = null
    try {
      const results = await adminAPI.usage.searchUsers(query)
      if (sequence === userSearchSequence) {
        syncSuggestionItems(
          userSuggestions,
          results.sort((a, b) => Number(a.deleted) - Number(b.deleted)),
          (user) => ({
            primaryText: user.email,
            secondaryText:
              [
                [user.notes, user.username].map((value) => value?.trim()).find(Boolean),
                user.deleted ? t('admin.usage.userDeletedBadge') : '',
              ].filter(Boolean).join(' · ') || `#${user.id}`,
          })
        )
      }
    } catch {
      if (sequence === userSearchSequence) {
        syncSuggestionItems(userSuggestions, [], () => ({ primaryText: '' }))
      }
    }
  }, 300)
}

const handleUserSearch = (value: string) => {
  userKeyword.value = value
  showUserDropdown.value = true
  if (filters.value.user_id != null) {
    filters.value.user_id = undefined
    clearApiKey()
    emitChange()
  }
  debounceUserSearch()
}

const onUserFocus = () => {
  showUserDropdown.value = true
  debounceUserSearch()
}

const onUserBlur = () => {
  showUserDropdown.value = false
}

const debounceApiKeySearch = () => {
  if (apiKeySearchTimeout) clearTimeout(apiKeySearchTimeout)
  apiKeySearchTimeout = setTimeout(async () => {
    try {
      syncSuggestionItems(
        apiKeySuggestions,
        await adminAPI.usage.searchApiKeys(
          filters.value.user_id,
          apiKeyKeyword.value || ''
        ),
        (key) => ({
          primaryText: key.name || String(key.id),
          secondaryText: `#${key.id}`,
        })
      )
    } catch {
      syncSuggestionItems(apiKeySuggestions, [], () => ({ primaryText: '' }))
    }
  }, 300)
}

const handleApiKeySearch = (value: string) => {
  apiKeyKeyword.value = value
  showApiKeyDropdown.value = true
  if (filters.value.api_key_id != null) {
    filters.value.api_key_id = undefined
    emitChange()
  }
  debounceApiKeySearch()
}

const selectUser = async (u: SimpleUser) => {
  userKeyword.value = u.email
  showUserDropdown.value = false
  filters.value.user_id = u.id
  clearApiKey()

  // Auto-load API keys for this user
  try {
    syncSuggestionItems(
      apiKeySuggestions,
      await adminAPI.usage.searchApiKeys(u.id, ''),
      (key) => ({
        primaryText: key.name || String(key.id),
        secondaryText: `#${key.id}`,
      })
    )
  } catch {
    syncSuggestionItems(apiKeySuggestions, [], () => ({ primaryText: '' }))
  }

  emitChange()
}

const selectUserOption = (option: SearchSuggestOption<SimpleUser>) => {
  void selectUser(option.value)
}

const clearUser = () => {
  userKeyword.value = ''
  syncSuggestionItems(userSuggestions, [], () => ({ primaryText: '' }))
  showUserDropdown.value = false
  filters.value.user_id = undefined
  clearApiKey()
  emitChange()
}

const selectApiKey = (k: SimpleApiKey) => {
  apiKeyKeyword.value = k.name || String(k.id)
  showApiKeyDropdown.value = false
  filters.value.api_key_id = k.id
  emitChange()
}

const selectApiKeyOption = (option: SearchSuggestOption<SimpleApiKey>) => {
  selectApiKey(option.value)
}

const clearApiKey = () => {
  apiKeyKeyword.value = ''
  syncSuggestionItems(apiKeySuggestions, [], () => ({ primaryText: '' }))
  showApiKeyDropdown.value = false
  filters.value.api_key_id = undefined
}

const onClearApiKey = () => {
  clearApiKey()
  emitChange()
}

const debounceAccountSearch = () => {
  if (accountSearchTimeout) clearTimeout(accountSearchTimeout)
  accountSearchTimeout = setTimeout(async () => {
    try {
      const res = await adminAPI.accounts.list(1, 20, { search: accountKeyword.value })
      syncSuggestionItems(
        accountSuggestions,
        res.items.map((a) => ({ id: a.id, name: a.name })),
        (account) => ({
          primaryText: account.name,
          secondaryText: `#${account.id}`,
        })
      )
    } catch {
      syncSuggestionItems(accountSuggestions, [], () => ({ primaryText: '' }))
    }
  }, 300)
}

const handleAccountSearch = (value: string) => {
  accountKeyword.value = value
  showAccountDropdown.value = true
  if (filters.value.account_id != null) {
    filters.value.account_id = undefined
    emitChange()
  }
  debounceAccountSearch()
}

const onAccountFocus = () => {
  showAccountDropdown.value = true
  debounceAccountSearch()
}

const onAccountBlur = () => {
  showAccountDropdown.value = false
}

const selectAccount = (a: SimpleAccount) => {
  accountKeyword.value = a.name
  showAccountDropdown.value = false
  filters.value.account_id = a.id
  emitChange()
}

const selectAccountOption = (option: SearchSuggestOption<SimpleAccount>) => {
  selectAccount(option.value)
}

const clearAccount = () => {
  accountKeyword.value = ''
  syncSuggestionItems(accountSuggestions, [], () => ({ primaryText: '' }))
  showAccountDropdown.value = false
  filters.value.account_id = undefined
  emitChange()
}

const onApiKeyFocus = () => {
  showApiKeyDropdown.value = true
  // Trigger search if no results yet
  if (apiKeySuggestions.value.length === 0) {
    debounceApiKeySearch()
  }
}

const onApiKeyBlur = () => {
  showApiKeyDropdown.value = false
}

const onDocumentClick = (e: MouseEvent) => {
  const target = e.target as Node | null
  if (!target) return

  const clickedInsideUser = userSearchRef.value?.contains(target) ?? false
  const clickedInsideApiKey = apiKeySearchRef.value?.contains(target) ?? false
  const clickedInsideAccount = accountSearchRef.value?.contains(target) ?? false

  if (!clickedInsideUser) showUserDropdown.value = false
  if (!clickedInsideApiKey) showApiKeyDropdown.value = false
  if (!clickedInsideAccount) showAccountDropdown.value = false
}

watch(
  () => props.startDate,
  (value) => {
    filters.value.start_date = value
  },
  { immediate: true }
)

watch(
  () => props.endDate,
  (value) => {
    filters.value.end_date = value
  },
  { immediate: true }
)

watch(
  () => filters.value.user_id,
  (userId) => {
    if (!userId) {
      syncSuggestionItems(userSuggestions, [], () => ({ primaryText: '' }))
    }
  }
)

watch(
  () => filters.value.api_key_id,
  (apiKeyId) => {
    if (!apiKeyId) {
      syncSuggestionItems(apiKeySuggestions, [], () => ({ primaryText: '' }))
    }
  }
)

watch(
  () => filters.value.account_id,
  (accountId) => {
    if (!accountId) {
      syncSuggestionItems(accountSuggestions, [], () => ({ primaryText: '' }))
    }
  }
)

onMounted(async () => {
  document.addEventListener('click', onDocumentClick)
  try {
    const gs = await adminAPI.groups.list(1, 1000)
    groupOptions.value.push(...gs.items.map((g: any) => ({ value: g.id, label: g.name })))
  } catch {
    // Ignore filter option loading errors (page still usable)
  }
})

onUnmounted(() => {
  document.removeEventListener('click', onDocumentClick)
  if (userSearchTimeout) clearTimeout(userSearchTimeout)
  if (apiKeySearchTimeout) clearTimeout(apiKeySearchTimeout)
  if (accountSearchTimeout) clearTimeout(accountSearchTimeout)
})

// 供外部(如用户排行下钻)在程序化设置 user_id 后回显选中的用户邮箱
const setUserKeyword = (email: string) => {
  userKeyword.value = email
  syncSuggestionItems(userSuggestions, [], () => ({ primaryText: '' }))
  showUserDropdown.value = false
}

const getUserSearchRevision = () => userSearchSequence

defineExpose({ getUserSearchRevision, setUserKeyword })
</script>
