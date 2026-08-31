import { performance } from "node:perf_hooks"
import { describe, expect, it } from "vitest"
import { redact, redactSensitivePaths } from "../src/redaction.js"

const longTextBytes = 64 * 1024
const redactionPerformanceBudgetMs = 1_000

describe("redact", () => {
  it("removes sensitive fields and bearer tokens recursively", () => {
    expect(redact({ apiKey: "sk-secret", nested: { authorization: "Bearer abc", safe: "Bearer abc.def" } }))
      .toEqual({ apiKey: "[REDACTED]", nested: { authorization: "[REDACTED]", safe: "Bearer [REDACTED]" } })
  })
  it("redacts credentials embedded in free text and secret form fields", () => {
    expect(redact({
      text: "password=hello https://alice:hunter2@example.com and \"apiKey\":\"sk-private-value\"",
      field: { type: "secret", id: "credential", value: "hidden", defaultValue: "also-hidden" },
    })).toEqual({
      text: "password=[REDACTED] https://[REDACTED]@example.com and \"apiKey\":[REDACTED]",
      field: { type: "secret", id: "credential", value: "[REDACTED]", defaultValue: "[REDACTED]" },
    })
  })
  it("redacts private key blocks without removing adjacent diagnostics", () => {
    const value = redact("read /srv/luna/key.pem: -----BEGIN PRIVATE KEY-----\nsecret-material\n-----END PRIVATE KEY-----: permission denied")
    expect(value).toContain("/srv/luna/key.pem")
    expect(value).toContain("permission denied")
    expect(value).toContain("[REDACTED PRIVATE KEY]")
    expect(value).not.toContain("secret-material")
  })
  it("masks generated secret values with equal-length asterisks", () => {
    expect(redact({ secrets: ["abc123", "qwertyuiop"], encoding: "base64", length: 6 }))
      .toEqual({ secrets: ["******", "**********"], encoding: "base64", length: 6 })
    expect(redact({ secrets: [] })).toEqual({ secrets: [] })
  })
  it("redacts values inside secret key-value fields", () => {
    const value = redact({
      field: {
        environment: [{ key: "CONFIG_VALUE", value: "database-secret" }],
      },
    })

    expect(value).toEqual({
      field: {
        environment: [{ key: "CONFIG_VALUE", value: "[REDACTED]" }],
      },
    })
    expect(JSON.stringify(value)).not.toContain("database-secret")
  })

  it("preserves non-credential token metrics and diagnostic identifiers", () => {
    expect(redact({
      "luna.context.input_tokens.estimated": 2048,
      outputTokenCount: 512,
      token: "credential-value",
      resourceId: "airun_123",
    })).toEqual({
      "luna.context.input_tokens.estimated": 2048,
      outputTokenCount: 512,
      token: "[REDACTED]",
      resourceId: "airun_123",
    })
  })
  it("keeps non-secrets fields intact next to generated secrets", () => {
    expect(redact({ secrets: ["s3cr3t"], encoding: "alphanumeric" }))
      .toEqual({ secrets: ["******"], encoding: "alphanumeric" })
  })

  it("processes long non-sensitive text without superlinear URL scanning", () => {
    const value = "a".repeat(longTextBytes)
    const startedAt = performance.now()
    const result = redact(value)
    const elapsedMs = performance.now() - startedAt

    expect(result).toBe(value)
    expect(elapsedMs).toBeLessThan(redactionPerformanceBudgetMs)
  })

  it("redacts credentials in long text without superlinear URL scanning", () => {
    const padding = "a".repeat(longTextBytes)
    const value = `${padding} https://alice:hunter2@example.com token=credential-value ${padding}`
    const startedAt = performance.now()
    const result = redact(value)
    const elapsedMs = performance.now() - startedAt

    expect(result).toBe(`${padding} https://[REDACTED]@example.com token=[REDACTED] ${padding}`)
    expect(elapsedMs).toBeLessThan(redactionPerformanceBudgetMs)
  })
})

describe("redactSensitivePaths", () => {
  it("masks only catalog paths without changing the arguments used for approval and execution", () => {
    const argumentsValue = {
      body: {
        command: "kubectl get secret",
        note: "Authorization: Bearer raw-token",
        items: [{ name: "PUBLIC_NAME", value: "runtime-secret" }],
      },
    }

    expect(redactSensitivePaths(argumentsValue, ["body.command", "body.items.*.value"])).toEqual({
      body: {
        command: "[REDACTED]",
        note: "Authorization: Bearer raw-token",
        items: [{ name: "PUBLIC_NAME", value: "[REDACTED]" }],
      },
    })
    expect(argumentsValue.body.command).toBe("kubectl get secret")
    expect(argumentsValue.body.items[0]?.value).toBe("runtime-secret")
  })

  it("also accepts JSON Pointer paths", () => {
    expect(redactSensitivePaths({ body: { "command/name": "secret" } }, ["/body/command~1name"]))
      .toEqual({ body: { "command/name": "[REDACTED]" } })
  })
})
