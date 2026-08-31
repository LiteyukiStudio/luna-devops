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
const privateKeyBlock = /-----BEGIN (?:[A-Z ]+ )?PRIVATE KEY-----[\s\S]*?-----END (?:[A-Z ]+ )?PRIVATE KEY-----/g

export function redact<T>(value: T): T {
  return visit(value, new WeakSet()) as T
}

export function redactSensitivePaths<T>(value: T, paths: readonly string[]): T {
  const result = structuredClone(value)
  for (const path of paths) maskPath(result, sensitivePathSegments(path), 0)
  return result
}

function sensitivePathSegments(path: string): string[] {
  if (!path.startsWith("/")) return path.split(".").filter(Boolean)
  return path.slice(1).split("/").filter(Boolean).map(segment => segment.replaceAll("~1", "/").replaceAll("~0", "~"))
}

function maskPath(value: unknown, segments: string[], index: number): void {
  if (!value || typeof value !== "object" || index >= segments.length) return
  const segment = segments[index]
  if (!segment) return
  if (segment === "*") {
    for (const item of Array.isArray(value) ? value : Object.values(value))
      maskPath(item, segments, index + 1)
    return
  }
  const record = value as Record<string, unknown>
  if (!Object.prototype.hasOwnProperty.call(record, segment)) return
  if (index === segments.length - 1) {
    record[segment] = "[REDACTED]"
    return
  }
  maskPath(record[segment], segments, index + 1)
}

function maskSecrets(item: unknown): unknown {
  if (typeof item === "string") return "*".repeat(item.length)
  if (Array.isArray(item)) return item.map(maskSecrets)
  if (!item || typeof item !== "object") return item
  return visit(item, new WeakSet())
}

function visit(value: unknown, seen: WeakSet<object>): unknown {
  if (typeof value === "string") {
    const redacted = value
      .replace(/Bearer\s+[A-Za-z0-9._~+/=-]+/gi, "Bearer [REDACTED]")
      .replace(/(sk-[A-Za-z0-9_-]{8})[A-Za-z0-9_-]+/g, "$1…[REDACTED]")
      .replace(sensitiveAssignment, "$1$2[REDACTED]")
    return redactUrlCredentials(redacted)
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

function redactUrlCredentials(value: string): string {
  let markerIndex = value.indexOf("://")
  if (markerIndex < 0) return value

  const fragments: string[] = []
  let copiedUntil = 0
  let redacted = false
  while (markerIndex >= 0) {
    if (!hasValidSchemeBefore(value, markerIndex)) {
      markerIndex = value.indexOf("://", markerIndex + 3)
      continue
    }

    const credentialsStart = markerIndex + 3
    let separatorIndex = credentialsStart
    while (separatorIndex < value.length && !isUsernameDelimiter(value.charCodeAt(separatorIndex))) {
      separatorIndex += 1
    }
    if (separatorIndex === credentialsStart || value.charCodeAt(separatorIndex) !== 0x3a) {
      markerIndex = value.indexOf("://", markerIndex + 3)
      continue
    }

    const passwordStart = separatorIndex + 1
    let credentialsEnd = passwordStart
    while (credentialsEnd < value.length && !isPasswordDelimiter(value.charCodeAt(credentialsEnd))) {
      credentialsEnd += 1
    }
    if (credentialsEnd === passwordStart || value.charCodeAt(credentialsEnd) !== 0x40) {
      markerIndex = value.indexOf("://", markerIndex + 3)
      continue
    }

    fragments.push(value.slice(copiedUntil, credentialsStart), "[REDACTED]@")
    copiedUntil = credentialsEnd + 1
    redacted = true
    markerIndex = value.indexOf("://", copiedUntil)
  }

  if (!redacted) return value
  fragments.push(value.slice(copiedUntil))
  return fragments.join("")
}

function hasValidSchemeBefore(value: string, markerIndex: number): boolean {
  let segmentStart = markerIndex
  while (segmentStart > 0 && isSchemeCharacter(value.charCodeAt(segmentStart - 1))) {
    segmentStart -= 1
  }
  while (segmentStart < markerIndex && !isAsciiLetter(value.charCodeAt(segmentStart))) {
    segmentStart += 1
  }
  return segmentStart < markerIndex
}

function isAsciiLetter(code: number): boolean {
  return code >= 0x41 && code <= 0x5a || code >= 0x61 && code <= 0x7a
}

function isSchemeCharacter(code: number): boolean {
  return isAsciiLetter(code)
    || code >= 0x30 && code <= 0x39
    || code === 0x2b
    || code === 0x2d
    || code === 0x2e
}

function isUsernameDelimiter(code: number): boolean {
  return isWhitespace(code) || code === 0x2f || code === 0x40 || code === 0x3a
}

function isPasswordDelimiter(code: number): boolean {
  return isWhitespace(code) || code === 0x2f || code === 0x40
}

function isWhitespace(code: number): boolean {
  return code >= 0x09 && code <= 0x0d
    || code === 0x20
    || code === 0xa0
    || code === 0x1680
    || code >= 0x2000 && code <= 0x200a
    || code === 0x2028
    || code === 0x2029
    || code === 0x202f
    || code === 0x205f
    || code === 0x3000
    || code === 0xfeff
}
