import type { ModelToolCall } from "../provider/provider.js"

const toolCallFinishReasons = new Set(["tool_calls", "function_call"])
const finalFinishReasons = new Set(["stop", "end_turn"])

export type ModelDecision =
  | { kind: "tool_calls"; calls: ModelToolCall[]; finishReason?: string }
  | { kind: "final"; text: string; finishReason?: string }
  | { kind: "empty"; finishReason?: string }
  | { kind: "protocol_error"; code: "missing_tool_calls" | "invalid_finish_reason" }

export type ModelStepResult = {
  answer: string
  toolCalls: ModelToolCall[]
  finishReason?: string
}

export function modelDecision(result: ModelStepResult): ModelDecision {
  const finishReason = result.finishReason
  const expectsToolCalls = finishReason !== undefined && toolCallFinishReasons.has(finishReason)
  if (result.toolCalls.length > 0) {
    if (finishReason !== undefined && !expectsToolCalls)
      return { kind: "protocol_error", code: "invalid_finish_reason" }
    return { kind: "tool_calls", calls: result.toolCalls, ...(finishReason === undefined ? {} : { finishReason }) }
  }
  if (expectsToolCalls) return { kind: "protocol_error", code: "missing_tool_calls" }
  if (finishReason !== undefined && !finalFinishReasons.has(finishReason))
    return { kind: "protocol_error", code: "invalid_finish_reason" }
  if (result.answer.trim())
    return { kind: "final", text: result.answer, ...(finishReason === undefined ? {} : { finishReason }) }
  return { kind: "empty", ...(finishReason === undefined ? {} : { finishReason }) }
}
