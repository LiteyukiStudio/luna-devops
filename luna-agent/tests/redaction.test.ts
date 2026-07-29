import { describe, expect, it } from "vitest"
import { redact } from "../src/redaction.js"

describe("redact", () => {
  it("removes sensitive fields and bearer tokens recursively", () => {
    expect(redact({ apiKey: "sk-secret", nested: { authorization: "Bearer abc", safe: "Bearer abc.def" } }))
      .toEqual({ apiKey: "[REDACTED]", nested: { authorization: "[REDACTED]", safe: "Bearer [REDACTED]" } })
  })
})
