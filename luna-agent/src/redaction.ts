const sensitiveKey = /authorization|cookie|token|secret|password|api[-_]?key|kubeconfig/i

export function redact<T>(value: T): T {
  return visit(value, new WeakSet()) as T
}

function visit(value: unknown, seen: WeakSet<object>): unknown {
  if (typeof value === "string") {
    return value
      .replace(/Bearer\s+[A-Za-z0-9._~+/=-]+/gi, "Bearer [REDACTED]")
      .replace(/(sk-[A-Za-z0-9_-]{8})[A-Za-z0-9_-]+/g, "$1…[REDACTED]")
  }
  if (!value || typeof value !== "object") return value
  if (seen.has(value)) return "[CIRCULAR]"
  seen.add(value)
  if (Array.isArray(value)) return value.map(item => visit(item, seen))
  return Object.fromEntries(Object.entries(value).map(([key, item]) => [
    key,
    sensitiveKey.test(key) ? "[REDACTED]" : visit(item, seen),
  ]))
}
