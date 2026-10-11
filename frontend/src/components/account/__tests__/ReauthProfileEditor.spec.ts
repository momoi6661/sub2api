import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { baseCompile } from '@intlify/message-compiler'
import ReauthProfileEditor from '../ReauthProfileEditor.vue'
import en from '@/i18n/locales/en/admin/accounts'
import zh from '@/i18n/locales/zh/admin/accounts'
import { testReauthScript } from '@/api/admin/reauthProfiles'

vi.mock('@/api/admin/reauthProfiles', () => ({
  readReauthScript: vi.fn().mockResolvedValue({ code_script: 'return { type: "code", code: "123456" };', custom_input: '' }),
  saveReauthProfile: vi.fn(), testReauthScript: vi.fn()
}))
vi.mock('@/api/admin/proxies', () => ({ getAll: vi.fn().mockResolvedValue([]) }))

describe('ReauthProfileEditor custom mode', () => {
  afterEach(() => { document.body.innerHTML = '' })
  for (const [locale, messages] of Object.entries({ en, zh })) {
    it(`${locale} renders custom inputs after selecting custom mode`, async () => {
      const renderErrors: unknown[] = []
      const wrapper = mount(ReauthProfileEditor, {
        attachTo: document.body,
        props: { show: false },
        global: {
          plugins: [createI18n({ legacy: false, messageCompiler: message => { if (typeof message !== 'string') throw new Error('Expected string'); return new Function('return ' + baseCompile(message, { mode: 'arrow' }).code)() }, locale, fallbackLocale: false, missingWarn: false, fallbackWarn: false, messages: { [locale]: { admin: messages } } })],
          config: { errorHandler: error => renderErrors.push(error) },
          stubs: { Icon: true }
        }
      })
      try {
        await wrapper.setProps({ show: true })
        await flushPromises()
        const mode = document.querySelector<HTMLButtonElement>('#auth-profile-mode')!
        mode.click()
        await flushPromises()
        const option = Array.from(document.querySelectorAll<HTMLElement>('[role="option"]')).find(el => el.textContent?.includes(locale === 'zh' ? '自定义' : 'Custom'))!
        expect(option).toBeTruthy()
        option.click()
        await flushPromises()
        expect(renderErrors).toEqual([])
        expect(document.querySelector('#auth-profile-script')).not.toBeNull()
        expect(document.querySelector('#auth-profile-test-input')).not.toBeNull()
        expect(document.querySelector('.modal-body')?.textContent).toContain('return { type: "auth_file", auth: authJson }')
        const returned = { type: 'auth_file', auth: JSON.stringify({ email: 'owner@example.com', access_token: 'test-token' }), custom: { items: [1, 2] } }
        vi.mocked(testReauthScript).mockResolvedValueOnce({ succeeded: true, result: returned })
        const testButton = Array.from(document.querySelectorAll<HTMLButtonElement>('.modal-body button')).find(el => el.textContent?.includes(messages.accounts.authProfiles.testCode))!
        testButton.click()
        await flushPromises()
        const output = document.querySelector('[role="status"] pre')!
        expect(JSON.parse(output.textContent!)).toEqual({ ...returned, auth: JSON.parse(returned.auth) })
        expect(output.classList.contains('max-h-72')).toBe(true)
        document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
        expect(wrapper.emitted('close')).toHaveLength(1)
      } finally { wrapper.unmount() }
    })
  }
})
