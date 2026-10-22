import { describe, expect, it } from "vitest"
import { modelDecision } from "../src/executor/decision.js"

const call = { operationId: "getProject", arguments: {} }

describe("模型决策协议", () => {
  it("把带工具调用的步骤判为 tool_calls，并接受各供应商的工具结束原因", () => {
    expect(modelDecision({ answer: "", toolCalls: [call], finishReason: "tool_calls" }))
      .toEqual({ kind: "tool_calls", calls: [call], finishReason: "tool_calls" })
    for (const finishReason of ["function_call", "tool_use"]) {
      expect(modelDecision({ answer: "正在查询。", toolCalls: [call], finishReason }))
        .toMatchObject({ kind: "tool_calls" })
    }
    expect(modelDecision({ answer: "", toolCalls: [call] })).toEqual({ kind: "tool_calls", calls: [call] })
  })

  it("可解析的工具调用不因结束原因取值被拒绝", () => {
    for (const finishReason of ["stop", "end_turn", "length", "content_filter", "unknown"]) {
      expect(modelDecision({ answer: "已完成。", toolCalls: [call], finishReason }))
        .toEqual({ kind: "tool_calls", calls: [call], finishReason })
    }
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
    for (const finishReason of ["tool_calls", "function_call", "tool_use"]) {
      expect(modelDecision({ answer: "我先检查部署状态。", toolCalls: [], finishReason }))
        .toEqual({ kind: "protocol_error", code: "missing_tool_calls" })
    }
    expect(modelDecision({ answer: "", toolCalls: [], finishReason: "function_call" }))
      .toEqual({ kind: "protocol_error", code: "missing_tool_calls" })
  })

  it("无工具调用时把长度截断判为 truncated，把内容过滤判为 filtered", () => {
    for (const finishReason of ["length", "max_tokens"]) {
      expect(modelDecision({ answer: "未完成的回答", toolCalls: [], finishReason })).toEqual({ kind: "truncated" })
      expect(modelDecision({ answer: "", toolCalls: [], finishReason })).toEqual({ kind: "truncated" })
    }
    expect(modelDecision({ answer: "被拦截的回答", toolCalls: [], finishReason: "content_filter" }))
      .toEqual({ kind: "filtered" })
  })

  it("未知结束原因按正常结束处理", () => {
    expect(modelDecision({ answer: "已完成。", toolCalls: [], finishReason: "unknown" }))
      .toEqual({ kind: "final", text: "已完成。", finishReason: "unknown" })
    expect(modelDecision({ answer: "", toolCalls: [], finishReason: "unknown" }))
      .toEqual({ kind: "empty", finishReason: "unknown" })
  })
})
