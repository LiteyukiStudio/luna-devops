export class DiagnosticError extends Error {
  constructor(
    readonly code: string,
    message: string,
    readonly hint: string,
    options?: ErrorOptions,
  ) {
    super(message, options)
    this.name = "DiagnosticError"
  }
}

export function findDiagnosticError(error: unknown): DiagnosticError | undefined {
  const seen = new Set<unknown>()
  let current = error
  while (current instanceof Error && !seen.has(current)) {
    if (current instanceof DiagnosticError) return current
    seen.add(current)
    current = current.cause
  }
  return undefined
}
