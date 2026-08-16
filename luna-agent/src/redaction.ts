const sensitiveKey = /authorization|cookie|token|secret|password|api[-_]?key|kubeconfig/i
const sensitiveAssignment = /(authorization|cookie|token|secret|password|api[-_]?key|kubeconfig)(["']?\s*[=:]\s*)("[^"]*"|'[^']*'|[^\s,;]+)/gi
const urlCredentials = /([a-z][a-z0-9+.-]*:\/\/)[^\s/@:]+:[^\s/@]+@/gi

export function redact<T>(value: T): T {
  return visit(value, new WeakSet()) as T
}

function maskSecrets(item: unknown): unknown {
  if (typeof item === "string") return "*".repeat(item.length)
  if (Array.isArray(item)) return item.map(maskSecrets)
  if (!item || typeof item !== "object") return item
  return visit(item, new WeakSet())
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
  const keyValuePair = typeof record.key === "string" && Object.prototype.hasOwnProperty.call(record, "value")
  return Object.fromEntries(Object.entries(record).map(([key, item]) => {
    // generateSecret 的生成值（secrets 数组）按等长 `*` 掩码，保留可辨识的位数信息；
    // 其余敏感键继续使用固定 [REDACTED] 占位。
    if (key === "secrets" && Array.isArray(item)) return [key, maskSecrets(item)]
    return [
      key,
      sensitiveKey.test(key) || keyValuePair && key === "value" || (secretContainer && /^(default)?value$/i.test(key))
        ? "[REDACTED]"
        : visit(item, seen),
    ]
  }))
}
