const sensitiveKeys = new Set([
  "authorization",
  "cookie",
  "setcookie",
  "password",
  "passwd",
  "secret",
  "token",
  "clientsecret",
  "apikey",
  "accesstoken",
  "refreshtoken",
  "idtoken",
  "privatekey",
  "kubeconfig",
])
const sensitiveAssignment = /\b(authorization|cookie|set-cookie|token|secret|password|passwd|client_secret|api[-_]?key|access_token|refresh_token|id_token|private_key|kubeconfig)(["']?\s*[=:]\s*)("[^"]*"|'[^']*'|[^\s,;]+)/gi
const urlCredentials = /([a-z][a-z0-9+.-]*:\/\/)[^\s/@:]+:[^\s/@]+@/gi
const privateKeyBlock = /-----BEGIN (?:[A-Z ]+ )?PRIVATE KEY-----[\s\S]*?-----END (?:[A-Z ]+ )?PRIVATE KEY-----/g

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
      .replace(privateKeyBlock, "[REDACTED PRIVATE KEY]")
  }
  if (!value || typeof value !== "object") return value
  if (seen.has(value)) return "[CIRCULAR]"
  seen.add(value)
  if (Array.isArray(value)) return value.map(item => visit(item, seen))
  const record = value as Record<string, unknown>
  const secretContainer = record.type === "secret" || record.valueMode === "secret"
  const keyValuePair = typeof record.key === "string" && Object.prototype.hasOwnProperty.call(record, "value")
  return Object.fromEntries(Object.entries(record).map(([key, item]) => {
    // 生成值数组按等长 `*` 掩码，保留可辨识的位数信息；
    // 其余敏感键继续使用固定 [REDACTED] 占位。
    if (key === "secrets" && Array.isArray(item)) return [key, maskSecrets(item)]
    return [
      key,
      isSensitiveKey(key) || keyValuePair && key === "value" || (secretContainer && /^(default)?value$/i.test(key))
        ? "[REDACTED]"
        : visit(item, seen),
    ]
  }))
}

function isSensitiveKey(key: string): boolean {
  return sensitiveKeys.has(key.replaceAll(/[-_.]/g, "").toLowerCase())
}
