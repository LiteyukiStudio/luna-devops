import { describe, expect, it } from "vitest"
import { serializeToolResultPayload, toolResultMessage } from "../src/executor/tool-results.js"

describe("tool result payload preservation", () => {
  it("passes through small and large results intact without local byte-budget truncation", () => {
    const items = Array.from({ length: 120 }, (_, index) => ({
      id: `item-${index}`,
      name: `名称-${index}`,
      description: "完整的工具返回描述文本".repeat(30),
      env: [{ key: "LOG_LEVEL", value: "info" }],
    }))
    const result = { status: "succeeded", result: { data: { items } } }
    const serialized = serializeToolResultPayload(result)
    expect(JSON.parse(serialized)).toEqual(result)
    expect(serialized).not.toContain("_truncated")

    const message = toolResultMessage({ id: "call_1", operationId: "listItems", arguments: {} }, {
      ...result,
      apiKey: "sk-secret-token-value",
    })
    expect(message.role).toBe("tool")
    expect(message.content).toContain('"LOG_LEVEL"')
    expect(message.content).toContain('"info"')
    expect(message.content).not.toContain("sk-secret-token-value")
  })
})
