export function normalizeWebConsoleOverride(value: unknown): boolean | null {
  if (value === false || value === 'false')
    return false
  return null
}

export function effectiveWebConsoleEnabled(projectDefault: boolean, targetOverride: unknown): boolean {
  return projectDefault && normalizeWebConsoleOverride(targetOverride) !== false
}
