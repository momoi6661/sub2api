/** Prefer a user label, then the authorized email; never use token text as a name. */
export function oauthAccountName(
  name: string,
  email: unknown,
  platform: string,
  index = 0,
  count = 1,
): string {
  const customName = name.trim()
  const authorizedEmail = typeof email === 'string' ? email.trim() : ''
  if (!customName && authorizedEmail) return authorizedEmail
  const baseName = customName || `${platform} OAuth Account`
  return count > 1 ? `${baseName} #${index + 1}` : baseName
}
