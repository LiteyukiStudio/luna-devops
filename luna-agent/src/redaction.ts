const sensitiveKey = /authorization|cookie|token|secret|password|api[-_]?key|kubeconfig/i
const sensitiveAssignment = /(authorization|cookie|token|secret|password|api[-_]?key|kubeconfig)(["']?\s*[=:]\s*)("[^"]*"|'[^']*'|[^\s,;]+)/gi
const urlCredentials = /([a-z][a-z0-9+.-]*:\/\/)[^\s/@:]+:[^\s/@]+@/gi

export function redact<T>(value: T): T {
  return visit(value, new WeakSet()) as T
}

function visit(value: unknown, seen: WeakSet<object>): unknown {
  if (typeof value === "string") {
    return value
      .replace(/Bearer\s+[A-Za-z0-9._~+/=-]+/gi, "Bearer [REDACTED]")
      .replace(/(sk-[A-Za-z0-9_-]{8})[A-Za-z0-9_-]+/g, "$1…[REDACTED]")
      .replace(sensitiveAssignment, "$1$2[REDACTED]")
      .replace(urlCredentials, "$1[REDACTED]@")
  }
  if (!value || typeof value !== "object") return value
  if (seen.has(value)) return "[CIRCULAR]"
  seen.add(value)
  if (Array.isArray(value)) return value.map(item => visit(item, seen))
  const record = value as Record<string, unknown>
  const secretContainer = record.type === "secret" || record.valueMode === "secret"
  return Object.fromEntries(Object.entries(record).map(([key, item]) => [
    key,
    sensitiveKey.test(key) || (secretContainer && /^(default)?value$/i.test(key))
      ? "[REDACTED]"
      : visit(item, seen),
  ]))
}
