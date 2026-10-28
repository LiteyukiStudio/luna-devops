import type { ModelToolCall } from "../provider/provider.js"

const toolCallFinishReasons = new Set(["tool_calls", "function_call", "tool_use"])
const truncatedFinishReasons = new Set(["length", "max_tokens"])
const filteredFinishReasons = new Set(["content_filter"])

export type ModelDecision =
  | { kind: "tool_calls"; calls: ModelToolCall[]; finishReason?: string }
  | { kind: "final"; text: string; finishReason?: string }
  | { kind: "empty"; finishReason?: string }
  | { kind: "truncated" }
  | { kind: "filtered" }
  | { kind: "protocol_error"; code: "missing_tool_calls" }

export type ModelStepResult = {
  answer: string
  toolCalls: ModelToolCall[]
  finishReason?: string
}

// 可解析的工具调用优先于结束原因：兼容网关常以 stop 结束工具调用步骤，结束原因只作遥测参考。
export function modelDecision(result: ModelStepResult): ModelDecision {
  const finishReason = result.finishReason
  const withFinishReason = finishReason === undefined ? {} : { finishReason }
  if (result.toolCalls.length > 0) return { kind: "tool_calls", calls: result.toolCalls, ...withFinishReason }
  if (finishReason !== undefined) {
    if (toolCallFinishReasons.has(finishReason)) return { kind: "protocol_error", code: "missing_tool_calls" }
    if (truncatedFinishReasons.has(finishReason)) return { kind: "truncated" }
    if (filteredFinishReasons.has(finishReason)) return { kind: "filtered" }
  }
  if (result.answer.trim()) return { kind: "final", text: result.answer, ...withFinishReason }
  return { kind: "empty", ...withFinishReason }
}
