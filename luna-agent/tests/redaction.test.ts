import { describe, expect, it } from "vitest"
import { redact } from "../src/redaction.js"

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
})
