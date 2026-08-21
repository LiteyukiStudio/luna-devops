import { describe, expect, it } from "vitest"
import { serializeToolResultPayload } from "../src/executor/tool-results.js"

describe("tool result payload bounding", () => {
  const testPayloadBudget = 24 * 1024

  it("passes through small results unchanged", () => {
    const result = { status: "succeeded", result: { data: { items: [{ id: "1", name: "a" }] } } }
    expect(serializeToolResultPayload(result)).toBe(JSON.stringify(result))
  })

  it("bounds a large list result and keeps valid JSON with a truncation marker", () => {
    const items = Array.from({ length: 100 }, (_, index) => ({
      id: `item-${index}`,
      name: `名称-${index}`,
      description: "很长的描述文本".repeat(30),
    }))
    const result = { status: "succeeded", result: { data: { items, truncated: true } } }
    const serialized = serializeToolResultPayload(result, testPayloadBudget)
    expect(() => { JSON.parse(serialized) }).not.toThrow()
    expect(Buffer.byteLength(serialized, "utf8")).toBeLessThanOrEqual(26_000)
    expect(serialized).toContain("_truncated")
  })

  it("bounds a very large single text payload", () => {
    const result = { status: "succeeded", result: { data: { text: "正文".repeat(50_000) } } }
    const serialized = serializeToolResultPayload(result, testPayloadBudget)
    expect(() => { JSON.parse(serialized) }).not.toThrow()
    expect(Buffer.byteLength(serialized, "utf8")).toBeLessThanOrEqual(26_000)
  })

})
