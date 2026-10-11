import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { getSubscription } from '@/api/admin/accounts'
import AccountSubscriptionExpiry from '../AccountSubscriptionExpiry.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string, params?: { days: number }) => params ? `${key}:${params.days}` : key }),
}))
vi.mock('@/api/admin/accounts', () => ({ getSubscription: vi.fn() }))

beforeEach(() => {
  vi.mocked(getSubscription).mockReset()
  vi.stubGlobal('IntersectionObserver', undefined)
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('AccountSubscriptionExpiry', () => {
  it('counts a partial day and updates to expired while the page stays open', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-11T00:00:00Z'))
    vi.mocked(getSubscription).mockResolvedValue({ subscription_expires_at: '2026-10-11T00:01:00Z', checked_at: '' })
    const wrapper = mount(AccountSubscriptionExpiry, { props: { accountId: 1 } })
    await flushPromises()
    expect(getSubscription).toHaveBeenCalledWith(1, false, expect.any(AbortSignal))
    expect(wrapper.text()).toContain('subscriptionRemainingDays:1')
    await vi.advanceTimersByTimeAsync(60_000)
    expect(wrapper.text()).toContain('subscriptionExpired')
    expect(wrapper.text()).not.toContain('subscriptionRemainingDays')
    wrapper.unmount()
  })

  it.each(['', 'invalid'])('shows unavailable instead of inventing an expiry for %s', async (expiresAt) => {
    vi.mocked(getSubscription).mockResolvedValue({ subscription_expires_at: expiresAt, checked_at: '' })
    const wrapper = mount(AccountSubscriptionExpiry, { props: { accountId: 1 } })
    await flushPromises()
    expect(wrapper.text()).toContain('subscriptionExpiryUnavailable')
    expect(wrapper.text()).not.toContain('subscriptionExpired')
    wrapper.unmount()
  })

  it('queries only visible accounts and ignores a late response for a previous account', async () => {
    let intersect!: IntersectionObserverCallback
    vi.stubGlobal('IntersectionObserver', class {
      constructor(callback: IntersectionObserverCallback) { intersect = callback }
      observe() {}
      disconnect() {}
    })
    let resolveOld!: (result: Awaited<ReturnType<typeof getSubscription>>) => void
    vi.mocked(getSubscription).mockReturnValueOnce(new Promise(resolve => { resolveOld = resolve }))
    vi.mocked(getSubscription).mockResolvedValue({ subscription_expires_at: '', checked_at: '' })
    const wrapper = mount(AccountSubscriptionExpiry, { props: { accountId: 1 } })
    expect(getSubscription).not.toHaveBeenCalled()
    intersect([{ isIntersecting: true }] as IntersectionObserverEntry[], {} as IntersectionObserver)
    expect(getSubscription).toHaveBeenCalledTimes(1)
    await wrapper.setProps({ accountId: 2 })
    await flushPromises()
    resolveOld({ subscription_expires_at: '2027-11-11T00:00:00Z', checked_at: '' })
    await flushPromises()
    expect(getSubscription).toHaveBeenLastCalledWith(2, false, expect.any(AbortSignal))
    expect(wrapper.text()).not.toContain('2027-11-11')
    expect(wrapper.text()).toContain('subscriptionExpiryUnavailable')
    wrapper.unmount()
  })

  it('forces a fresh upstream query on list refresh and has no independent polling', async () => {
    vi.useFakeTimers()
    vi.mocked(getSubscription).mockResolvedValue({ subscription_expires_at: '', checked_at: '' })
    const wrapper = mount(AccountSubscriptionExpiry, { props: { accountId: 1, refreshKey: 0 } })
    await flushPromises()
    await wrapper.setProps({ refreshKey: 1 })
    await flushPromises()
    expect(getSubscription).toHaveBeenCalledTimes(2)
    expect(getSubscription).toHaveBeenLastCalledWith(1, true, expect.any(AbortSignal))
    await vi.advanceTimersByTimeAsync(300_000)
    expect(getSubscription).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('does not query with the option off and preserves the result when disabled', async () => {
    vi.mocked(getSubscription).mockResolvedValue({ subscription_expires_at: '2027-01-01T00:00:00Z', checked_at: '' })
    const wrapper = mount(AccountSubscriptionExpiry, { props: { accountId: 1, enabled: false, refreshKey: 0 } })
    await flushPromises()
    await wrapper.setProps({ refreshKey: 1 })
    expect(getSubscription).not.toHaveBeenCalled()
    await wrapper.setProps({ enabled: true })
    await flushPromises()
    expect(getSubscription).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('subscriptionExpiryDate')
    await wrapper.setProps({ enabled: false, refreshKey: 2 })
    await flushPromises()
    expect(getSubscription).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('subscriptionExpiryDate')
    wrapper.unmount()
  })
})
