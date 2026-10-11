import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AccountSubscriptionExpiry from '../AccountSubscriptionExpiry.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string, params?: { days: number }) => params ? `${key}:${params.days}` : key }),
}))

afterEach(() => vi.useRealTimers())

describe('AccountSubscriptionExpiry', () => {
  it('counts a partial day and updates to expired while the page stays open', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-11T00:00:00Z'))
    const wrapper = mount(AccountSubscriptionExpiry, { props: { expiresAt: '2026-10-11T00:01:00Z' } })
    expect(wrapper.text()).toContain('subscriptionRemainingDays:1')
    await vi.advanceTimersByTimeAsync(60_000)
    expect(wrapper.text()).toContain('subscriptionExpired')
    expect(wrapper.text()).not.toContain('subscriptionRemainingDays')
    wrapper.unmount()
  })

  it.each([undefined, 'invalid'])('shows unavailable instead of inventing an expiry for %s', (expiresAt) => {
    const wrapper = mount(AccountSubscriptionExpiry, { props: { expiresAt } })
    expect(wrapper.text()).toContain('subscriptionExpiryUnavailable')
    expect(wrapper.text()).not.toContain('subscriptionExpired')
    wrapper.unmount()
  })
})
