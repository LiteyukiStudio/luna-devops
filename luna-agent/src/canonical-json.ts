import { createHash } from "node:crypto"

export function canonicalJSONStringify(value: unknown): string {
  if (value === null || typeof value === "string" || typeof value === "boolean")
    return JSON.stringify(value)
  if (typeof value === "number") {
    if (!Number.isFinite(value))
      throw new Error("ai.invalid_tool_arguments")
    return JSON.stringify(value)
  }
  if (Array.isArray(value))
    return `[${value.map(canonicalJSONStringify).join(",")}]`
  if (typeof value === "object") {
    const record = value as Record<string, unknown>
    const entries = Object.keys(record)
      .filter(key => record[key] !== undefined)
      .sort()
      .map(key => `${JSON.stringify(key)}:${canonicalJSONStringify(record[key])}`)
    return `{${entries.join(",")}}`
  }
  throw new Error("ai.invalid_tool_arguments")
}

export function hashCanonicalJSON(value: unknown): string {
  return `sha256:${createHash("sha256").update(canonicalJSONStringify(value)).digest("hex")}`
}
