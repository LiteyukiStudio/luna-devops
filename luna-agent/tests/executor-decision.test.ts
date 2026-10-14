import { describe, expect, it } from "vitest"
import { modelDecision } from "../src/executor/decision.js"

const call = { operationId: "getProject", arguments: {} }

describe("模型决策协议", () => {
  it("把带工具调用的步骤判为 tool_calls，并接受两种工具结束原因", () => {
    expect(modelDecision({ answer: "", toolCalls: [call], finishReason: "tool_calls" }))
      .toEqual({ kind: "tool_calls", calls: [call], finishReason: "tool_calls" })
    expect(modelDecision({ answer: "正在查询。", toolCalls: [call], finishReason: "function_call" }))
      .toMatchObject({ kind: "tool_calls" })
    expect(modelDecision({ answer: "", toolCalls: [call] })).toEqual({ kind: "tool_calls", calls: [call] })
  })

  it("把无工具调用的非空文本判为 final，并保留原始文本", () => {
    expect(modelDecision({ answer: " 已完成。 ", toolCalls: [], finishReason: "stop" }))
      .toEqual({ kind: "final", text: " 已完成。 ", finishReason: "stop" })
    expect(modelDecision({ answer: "已完成。", toolCalls: [] }))
      .toEqual({ kind: "final", text: "已完成。" })
    expect(modelDecision({ answer: "已完成。", toolCalls: [], finishReason: "end_turn" }))
      .toEqual({ kind: "final", text: "已完成。", finishReason: "end_turn" })
  })

  it("把无工具调用的空白文本判为 empty", () => {
    expect(modelDecision({ answer: "   ", toolCalls: [] })).toEqual({ kind: "empty" })
    expect(modelDecision({ answer: "", toolCalls: [], finishReason: "stop" }))
      .toEqual({ kind: "empty", finishReason: "stop" })
  })

  it("结束原因表示需要工具但没有工具调用时判为 missing_tool_calls", () => {
    expect(modelDecision({ answer: "我先检查部署状态。", toolCalls: [], finishReason: "tool_calls" }))
      .toEqual({ kind: "protocol_error", code: "missing_tool_calls" })
    expect(modelDecision({ answer: "", toolCalls: [], finishReason: "function_call" }))
      .toEqual({ kind: "protocol_error", code: "missing_tool_calls" })
  })

  it("工具调用与非工具结束原因同时出现时判为 invalid_finish_reason", () => {
    expect(modelDecision({ answer: "已完成。", toolCalls: [call], finishReason: "stop" }))
      .toEqual({ kind: "protocol_error", code: "invalid_finish_reason" })
    expect(modelDecision({ answer: "", toolCalls: [call], finishReason: "length" }))
      .toEqual({ kind: "protocol_error", code: "invalid_finish_reason" })
  })

  it("无工具调用时拒绝截断、过滤和未知结束原因", () => {
    for (const finishReason of ["length", "content_filter", "unknown"]) {
      expect(modelDecision({ answer: "未完成的回答", toolCalls: [], finishReason }))
        .toEqual({ kind: "protocol_error", code: "invalid_finish_reason" })
      expect(modelDecision({ answer: "", toolCalls: [], finishReason }))
        .toEqual({ kind: "protocol_error", code: "invalid_finish_reason" })
    }
  })
})
