export function buildRunIdFromHash(hash: string) {
  const params = new URLSearchParams(hash.replace(/^#/, ''))
  return params.get('buildRunId')?.trim() ?? ''
}
