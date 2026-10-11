import { describe, expect, it } from 'vitest'
import { baseCompile } from '@intlify/message-compiler'
import en from '../locales/en/admin/accounts'
import zh from '../locales/zh/admin/accounts'

describe('authorization editor messages', () => {
  for (const [locale, messages] of Object.entries({ en, zh })) {
    it(`${locale} custom mode messages compile without render errors`, () => {
      const errors: string[] = []
      for (const [key, message] of Object.entries(messages.accounts.authProfiles)) {
        if (typeof message === 'string') {
          baseCompile(message, { onError: error => errors.push(`${key}: ${error.message}`) })
        }
      }
      expect(errors).toEqual([])
    })
  }
})
