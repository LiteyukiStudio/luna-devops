import { describe, expect, it } from "vitest"
import { recentEmptyReadResults, requestsExplicitLiveRefresh } from "../src/executor/conversation-loop-state.js"

const now = Date.parse("2026-08-20T08:00:30.000Z")

function history(result: unknown, createdAt = "2026-08-20T08:00:15.000Z") {
  return [{
    turnIndex: 1,
    user: "查询资源",
    assistant: "没有资源",
    toolInteractions: [{
      type: "tool_call",
      status: "completed",
      createdAt,
      content: {
        operationId: "listRuntimeClusterResources",
        status: "succeeded",
        argumentsHash: "sha256:args",
        result,
      },
    }],
  }]
}

describe("conversation loop state", () => {
  it("inherits a recent empty delegated business result", () => {
    const result = {
      operationId: "listRuntimeClusterResources",
      verified: true,
      result: { items: [], total: 0 },
    }
    expect(recentEmptyReadResults(history(result), now)).toEqual([{
      operationId: "listRuntimeClusterResources",
      argumentsHash: "sha256:args",
      result,
    }])
  })

  it("does not block non-empty observations or stale empty observations", () => {
    expect(recentEmptyReadResults(history({ items: [{ name: "api" }], total: 1 }), now)).toEqual([])
    expect(recentEmptyReadResults(history({ items: [], total: 0 }, "2026-08-20T07:53:00.000Z"), now)).toEqual([])
  })

  it("only bypasses inherited empty results for an explicit live-state refresh", () => {
    expect(requestsExplicitLiveRefresh("请强制刷新实时状态，不要使用缓存结论")).toBe(true)
    expect(requestsExplicitLiveRefresh("再查一次刚才的空列表")).toBe(false)
  })
})
