import { defineComponent, onMounted, watch } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getBatchUsage,
  getUsage,
  getById,
  refreshOpenAIQuota,
  getBatchTodayStats,
  getUpstreamBillingProbeSettings,
  getAllProxies,
  getAllGroups
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchUsage: vi.fn(),
  getUsage: vi.fn(),
  getById: vi.fn(),
  refreshOpenAIQuota: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getUpstreamBillingProbeSettings: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getBatchUsage,
      getUsage,
      getById,
      refreshOpenAIQuota,
      getBatchTodayStats,
      getUpstreamBillingProbeSettings,
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      toggleSchedulable: vi.fn()
    },
    proxies: {
      getAll: getAllProxies
    },
    groups: {
      getAll: getAllGroups
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    token: 'test-token'
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const DataTableStub = {
  props: ['data', 'loading'],
  template: `
    <div data-test="data-table">
      <div v-if="loading" data-test="loading">loading</div>
      <div v-else v-for="row in data" :key="row.id" data-test="row">
        <span data-test="account-name">{{ row.name }}</span>
        <slot name="cell-usage" :row="row" />
        <slot name="cell-actions" :row="row" />
      </div>
    </div>
  `
}

const DataTableWithoutUsageStub = {
  props: ['data', 'loading'],
  template: `
    <div data-test="data-table">
      <div v-if="loading" data-test="loading">loading</div>
      <div v-else v-for="row in data" :key="row.id" data-test="row">
        <span data-test="account-name">{{ row.name }}</span>
        <slot name="cell-actions" :row="row" />
      </div>
    </div>
  `
}

const PaginationStub = {
  emits: ['update:page'],
  template: '<button data-test="next-page" @click="$emit(\'update:page\', 2)">next page</button>'
}

const AccountTableActionsStub = {
  emits: ['refresh'],
  template: '<button data-test="refresh-accounts" @click="$emit(\'refresh\')">refresh</button>'
}

const BulkEditAccountModalStub = {
  emits: ['updated'],
  template: '<button data-test="bulk-updated" @click="$emit(\'updated\')">updated</button>'
}

const EditAccountModalStub = defineComponent({
  props: {
    show: { type: Boolean, default: false },
    account: { type: Object, default: null }
  },
  template: '<div v-if="show" data-test="edit-modal">{{ account?.name }}</div>'
})

const usageCellMountedTokens: number[] = []
const usageCellRefreshTransitions: Array<[number | undefined, number | undefined]> = []

const AccountUsageCellStub = defineComponent({
  emits: ['runtime-state-updated', 'account-updated', 'usage-loaded'],
  props: {
    account: { type: Object, required: true },
    todayStats: { type: Object, default: null },
    todayStatsLoading: { type: Boolean, default: false },
    manualRefreshToken: { type: Number, default: 0 },
    batchedUsage: { type: Object, default: null },
    batchedUsageError: { type: String, default: null },
    batchedUsageLoading: { type: Boolean, default: false },
    requestBatchedUsage: { type: Function, default: null }
  },
  setup(props) {
    onMounted(() => {
      usageCellMountedTokens.push(props.manualRefreshToken)
    })
    watch(
      () => props.manualRefreshToken,
      (nextToken, prevToken) => {
        usageCellRefreshTransitions.push([prevToken, nextToken])
      }
    )
    return {}
  },
  template: `
    <div data-test="usage-cell">{{ manualRefreshToken }}</div>
    <button data-test="runtime-state-updated" @click="$emit('runtime-state-updated')">runtime state updated</button>
  `
})

const createAccount = (id: number, name: string) => ({
  id,
  name,
  platform: 'anthropic',
  type: 'oauth',
  status: 'active',
  schedulable: true,
  created_at: '2026-07-07T00:00:00Z',
  updated_at: '2026-07-07T00:00:00Z'
})

const pageResponse = (items: Array<Record<string, unknown>>) => ({
  items,
  total: items.length,
  page: 1,
  page_size: 20,
  pages: 1
})

const mountView = (renderUsage = true, pagination = false) =>
  mount(AccountsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: {
          template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
        },
        DataTable: renderUsage ? DataTableStub : DataTableWithoutUsageStub,
        Pagination: pagination ? PaginationStub : true,
        ConfirmDialog: true,
        AccountTableActions: AccountTableActionsStub,
        AccountTableFilters: { template: '<div></div>' },
        AccountBulkActionsBar: true,
        AccountActionMenu: true,
        ImportDataModal: true,
        ReAuthAccountModal: true,
        AccountTestModal: true,
        AccountStatsModal: true,
        ScheduledTestsPanel: true,
        SyncFromCrsModal: true,
        TempUnschedStatusModal: true,
        ErrorPassthroughRulesModal: true,
        TLSFingerprintProfilesModal: true,
        CreateAccountModal: true,
        EditAccountModal: EditAccountModalStub,
        BulkEditAccountModal: BulkEditAccountModalStub,
        PlatformTypeBadge: true,
        AccountCapacityCell: true,
        AccountStatusIndicator: true,
        AccountTodayStatsCell: true,
        AccountGroupsCell: true,
        AccountUsageCell: AccountUsageCellStub,
        Icon: true
      }
    }
  })

describe('admin AccountsView manual usage refresh', () => {
  beforeEach(() => {
    localStorage.clear()
    usageCellMountedTokens.length = 0
    usageCellRefreshTransitions.length = 0

    listAccounts.mockReset()
    listWithEtag.mockReset()
    getBatchUsage.mockReset()
    getUsage.mockReset()
    getById.mockReset()
    refreshOpenAIQuota.mockReset()
    getBatchTodayStats.mockReset()
    getUpstreamBillingProbeSettings.mockReset()
    getAllProxies.mockReset()
    getAllGroups.mockReset()

    listWithEtag.mockResolvedValue({ notModified: true, etag: null, data: null })
    getBatchUsage.mockResolvedValue({ usage: {}, errors: {} })
    getUsage.mockResolvedValue({})
    getById.mockImplementation(async (accountID: number) => createAccount(accountID, `account-${accountID}`))
    refreshOpenAIQuota.mockResolvedValue({})
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getUpstreamBillingProbeSettings.mockResolvedValue({ enabled: false })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
  })

  it('refreshes current-page usage cells when entering the account list', async () => {
    listAccounts
      .mockResolvedValueOnce(pageResponse([createAccount(1, 'before-refresh')]))

    const wrapper = mountView()
    await flushPromises()

    expect(usageCellMountedTokens).toEqual([0])
    expect(usageCellRefreshTransitions).toContainEqual([0, 1])
    expect(listAccounts).toHaveBeenCalledWith(
      1,
      20,
      expect.not.objectContaining({ refresh_usage: 'true' }),
      expect.anything()
    )
    expect(getUsage).toHaveBeenCalledWith(1, 'active')

    wrapper.unmount()
  })

  it('refreshes remounted current-page usage cells after the table reload finishes', async () => {
    listAccounts
      .mockResolvedValueOnce(pageResponse([createAccount(1, 'before-refresh')]))
      .mockResolvedValueOnce(pageResponse([createAccount(1, 'after-refresh')]))

    const wrapper = mountView()
    await flushPromises()

    expect(usageCellMountedTokens).toEqual([0])
    expect(usageCellRefreshTransitions).toContainEqual([0, 1])

    await wrapper.get('[data-test="refresh-accounts"]').trigger('click')
    await flushPromises()

    expect(usageCellMountedTokens).toEqual([0, 1])
    expect(usageCellRefreshTransitions).toContainEqual([1, 2])

    wrapper.unmount()
  })

  it('refreshes current-page usage cells after account reload actions finish', async () => {
    listAccounts
      .mockResolvedValueOnce(pageResponse([createAccount(1, 'before-reload')]))
      .mockResolvedValueOnce(pageResponse([createAccount(1, 'after-reload')]))

    const wrapper = mountView()
    await flushPromises()

    expect(usageCellMountedTokens).toEqual([0])

    await wrapper.get('[data-test="bulk-updated"]').trigger('click')
    await flushPromises()

    expect(usageCellMountedTokens).toEqual([0, 1])
    expect(usageCellRefreshTransitions).toContainEqual([1, 2])

    wrapper.unmount()
  })

  it('reloads current rows when an upstream quota query updates runtime state', async () => {
    listAccounts.mockResolvedValueOnce(pageResponse([{ ...createAccount(1, 'before-quota-query'), status: 'error', schedulable: false }]))
    listWithEtag.mockResolvedValueOnce({
      notModified: false,
      etag: 'updated-runtime-state',
      data: pageResponse([{ ...createAccount(1, 'after-quota-query'), status: 'active', schedulable: true }])
    })

    const wrapper = mountView()
    await flushPromises()
    listWithEtag.mockClear()

    await wrapper.get('[data-test="runtime-state-updated"]').trigger('click')
    await flushPromises()

    expect(listWithEtag).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('after-quota-query')

    wrapper.unmount()
  })

  it('force-queries every current-page account individually without replacing an open edit modal', async () => {
    listAccounts.mockResolvedValueOnce(pageResponse([
      createAccount(1, 'anthropic-oauth'),
      { ...createAccount(2, 'anthropic-setup-token'), type: 'setup-token' },
      { ...createAccount(3, 'unsupported-key'), type: 'apikey' }
    ]))
    let releaseFirstUsage: (() => void) | undefined
    getUsage
      .mockImplementationOnce(() => new Promise(resolve => { releaseFirstUsage = () => resolve({}) }))
      .mockResolvedValue({})

    const wrapper = mountView(false)
    await flushPromises()

    expect(getUsage).toHaveBeenCalledTimes(1)
    expect(getUsage).toHaveBeenCalledWith(1, 'active')
    expect(getBatchUsage).not.toHaveBeenCalled()

    getById.mockResolvedValueOnce(createAccount(1, 'anthropic-oauth'))
    await wrapper.get('[data-test="row"] button').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-test="edit-modal"]').text()).toBe('anthropic-oauth')

    releaseFirstUsage?.()
    await flushPromises()

    expect(getUsage).toHaveBeenNthCalledWith(2, 2, 'active')
    expect(getUsage).toHaveBeenNthCalledWith(3, 3, 'active')
    expect(wrapper.get('[data-test="edit-modal"]').text()).toBe('anthropic-oauth')
    wrapper.unmount()
  })

  it('refreshes an OpenAI quota snapshot before its active usage window', async () => {
    const calls: string[] = []
    listAccounts.mockResolvedValueOnce(pageResponse([
      { ...createAccount(1, 'openai-oauth'), platform: 'openai' }
    ]))
    refreshOpenAIQuota.mockImplementationOnce(async () => {
      calls.push('quota')
      return {}
    })
    getUsage.mockImplementationOnce(async () => {
      calls.push('usage')
      return {}
    })

    const wrapper = mountView(false)
    await flushPromises()

    expect(calls).toEqual(['quota', 'usage'])
    expect(getUsage).toHaveBeenCalledWith(1, 'active')
    wrapper.unmount()
  })

  it('refreshes OpenAI quota and the active usage window after an automatic list refresh', async () => {
    vi.useFakeTimers()
    localStorage.setItem('account-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 5 }))
    listAccounts.mockResolvedValueOnce(pageResponse([
      { ...createAccount(1, 'openai-oauth'), platform: 'openai' }
    ]))
    listWithEtag.mockResolvedValue({ notModified: true, etag: 'unchanged', data: null })

    const wrapper = mountView(false)
    await flushPromises()
    refreshOpenAIQuota.mockClear()
    getUsage.mockClear()

    await vi.advanceTimersByTimeAsync(6_000)
    await flushPromises()

    // Runtime-state reconciliation can perform another ETag list read. The
    // automatic refresh is verified by its required upstream usage calls.
    expect(listWithEtag).toHaveBeenCalled()
    expect(refreshOpenAIQuota).toHaveBeenCalledWith(1)
    expect(getUsage).toHaveBeenCalledWith(1, 'active')

    wrapper.unmount()
    vi.useRealTimers()
  })

  it('force-queries the new current page after pagination changes', async () => {
    listAccounts
      .mockResolvedValueOnce({ ...pageResponse([createAccount(1, 'first-page')]), total: 2, pages: 2 })
      .mockResolvedValueOnce({ ...pageResponse([createAccount(2, 'second-page')]), total: 2, page: 2, pages: 2 })

    const wrapper = mountView(false, true)
    await flushPromises()
    expect(getUsage).toHaveBeenLastCalledWith(1, 'active')

    await wrapper.get('[data-test="next-page"]').trigger('click')
    await flushPromises()

    expect(getUsage).toHaveBeenLastCalledWith(2, 'active')
    expect(getBatchUsage).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('continues querying later current-page accounts after one usage request fails', async () => {
    listAccounts.mockResolvedValueOnce(pageResponse([
      createAccount(1, 'first-account'),
      createAccount(2, 'failed-account'),
      createAccount(3, 'last-account')
    ]))
    getUsage
      .mockResolvedValueOnce({})
      .mockRejectedValueOnce(new Error('usage unavailable'))
      .mockResolvedValueOnce({})

    const wrapper = mountView(false)
    await flushPromises()

    expect(getUsage).toHaveBeenCalledTimes(3)
    expect(getUsage.mock.calls.map(([id]) => id)).toEqual([1, 2, 3])
    wrapper.unmount()
  })
})
