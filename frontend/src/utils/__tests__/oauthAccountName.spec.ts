import { describe, expect, it } from 'vitest'
import { oauthAccountName } from '../oauthAccountName'

describe('oauthAccountName', () => {
  it('uses a trimmed authorized email when no custom name is supplied', () => {
    expect(oauthAccountName('  ', ' user@example.com ', 'OpenAI')).toBe('user@example.com')
  })
  it('preserves a custom name', () => {
    expect(oauthAccountName(' My account ', 'user@example.com', 'OpenAI')).toBe('My account')
  })
  it('uses each email without numbering in a batch', () => {
    expect(oauthAccountName('', 'first@example.com', 'OpenAI', 0, 2)).toBe('first@example.com')
    expect(oauthAccountName('', 'second@example.com', 'OpenAI', 1, 2)).toBe('second@example.com')
  })
  it('numbers custom batch labels and fallback names', () => {
    expect(oauthAccountName('Team', 'user@example.com', 'OpenAI', 1, 2)).toBe('Team #2')
    expect(oauthAccountName('', undefined, 'OpenAI', 0, 2)).toBe('OpenAI OAuth Account #1')
  })
  it.each([undefined, null, '', '  ', 42, {}])('handles a missing or invalid email safely: %j', (email) => {
    expect(oauthAccountName('', email, 'OpenAI')).toBe('OpenAI OAuth Account')
  })
})
