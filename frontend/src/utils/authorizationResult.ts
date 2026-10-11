export function formatAuthorizationResult(result: unknown): string {
  const format = (_key: string, value: unknown): unknown => {
    if (typeof value === 'string') {
      try {
        const parsed: unknown = JSON.parse(value)
        if (parsed !== null && typeof parsed === 'object') return parsed
      } catch { /* Display ordinary strings unchanged. */ }
    }
    return value
  }
  return JSON.stringify(result, format, 2) ?? ''
}
