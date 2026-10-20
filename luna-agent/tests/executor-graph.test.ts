import { describe, expect, it } from "vitest"
import type { ModelDecision } from "../src/executor/decision.js"
import {
  assertGraphTransition,
  decisionAction,
  maxProtocolRepairs,
  resumeAction,
  RunGraphTransitionError,
  toolBatchAction,
} from "../src/executor/graph.js"

const toolCalls: Extract<ModelDecision, { kind: "tool_calls" }> = {
  kind: "tool_calls",
  calls: [{ id: "call_1", operationId: "getProject", arguments: {} }],
}

describe("Run 状态图", () => {
  it("把模型决策映射为唯一的下一步动作", () => {
    expect(decisionAction(toolCalls, { protocolRepairCount: 0, cardRepairExhausted: false }))
      .toEqual({ type: "execute_tools", calls: toolCalls.calls })
    expect(decisionAction({ kind: "final", text: "完成" }, { protocolRepairCount: 0, cardRepairExhausted: false }))
      .toEqual({ type: "finalize", text: "完成" })
    expect(decisionAction({ kind: "empty" }, { protocolRepairCount: 0, cardRepairExhausted: false }))
      .toEqual({ type: "retry" })
  })

  it("协议修复有次数上限，非法结束原因直接失败", () => {
    expect(decisionAction({ kind: "protocol_error", code: "missing_tool_calls" }, { protocolRepairCount: 0, cardRepairExhausted: false }))
      .toEqual({ type: "repair" })
    expect(decisionAction({ kind: "protocol_error", code: "missing_tool_calls" }, { protocolRepairCount: maxProtocolRepairs, cardRepairExhausted: false }))
      .toEqual({ type: "fail", errorCode: "ai.model_protocol_violation" })
    expect(decisionAction({ kind: "protocol_error", code: "invalid_finish_reason" }, { protocolRepairCount: 0, cardRepairExhausted: false }))
      .toEqual({ type: "fail", errorCode: "ai.model_protocol_violation" })
  })

  it("卡片修复耗尽后只允许最终答复或空答复", () => {
    expect(decisionAction({ kind: "final", text: "卡片未能生成" }, { protocolRepairCount: 0, cardRepairExhausted: true }))
      .toEqual({ type: "finalize", text: "卡片未能生成" })
    expect(decisionAction(toolCalls, { protocolRepairCount: 0, cardRepairExhausted: true }))
      .toEqual({ type: "fail", errorCode: "ai.interaction_card_schema_invalid" })
    expect(decisionAction({ kind: "protocol_error", code: "missing_tool_calls" }, { protocolRepairCount: 0, cardRepairExhausted: true }))
      .toEqual({ type: "fail", errorCode: "ai.model_protocol_violation" })
  })

  it("工具批次只返回继续、等待审批或完成三种控制流", () => {
    expect(toolBatchAction({ kind: "continue" })).toEqual({ type: "continue_model" })
    expect(toolBatchAction({ kind: "waiting_approval", toolCallId: "aitool_1" }))
      .toEqual({ type: "waiting_approval", toolCallId: "aitool_1" })
    expect(toolBatchAction({ kind: "complete" })).toEqual({ type: "complete" })
  })

  it("checkpoint 恢复计划映射为续跑动作", () => {
    expect(resumeAction({ kind: "model" }, "model")).toEqual({ type: "call_model" })
    expect(resumeAction({ kind: "settle_tool_batch" }, "tool_execution")).toEqual({ type: "settle_tools" })
    expect(resumeAction({ kind: "await_approval", toolCallId: "aitool_1" }, "awaiting_approval"))
      .toEqual({ type: "waiting_approval", toolCallId: "aitool_1" })
    expect(resumeAction({ kind: "finalize", finalOutput: "已决定" }, "finalizing"))
      .toEqual({ type: "finalize", finalOutput: "已决定" })
  })

  it("每个阶段只允许声明的动作", () => {
    expect(() => assertGraphTransition("model", { type: "call_model" })).not.toThrow()
    expect(() => assertGraphTransition("model", { type: "execute_tools", calls: [] })).not.toThrow()
    expect(() => assertGraphTransition("tool_execution", { type: "continue_model" })).not.toThrow()
    expect(() => assertGraphTransition("tool_execution", { type: "settle_tools" })).not.toThrow()
    expect(() => assertGraphTransition("awaiting_input", { type: "call_model" })).not.toThrow()
    expect(() => assertGraphTransition("awaiting_approval", { type: "waiting_approval", toolCallId: "aitool_1" })).not.toThrow()
    expect(() => assertGraphTransition("finalizing", { type: "finalize", text: "完成" })).not.toThrow()

    expect(() => assertGraphTransition("tool_execution", { type: "finalize", text: "完成" }))
      .toThrow(RunGraphTransitionError)
    expect(() => assertGraphTransition("awaiting_input", { type: "execute_tools", calls: [] }))
      .toThrow(RunGraphTransitionError)
    expect(() => assertGraphTransition("awaiting_approval", { type: "call_model" }))
      .toThrow(RunGraphTransitionError)
    expect(() => assertGraphTransition("finalizing", { type: "continue_model" }))
      .toThrow(RunGraphTransitionError)
  })

  it("决策、工具批次和恢复动作都按当前相位校验", () => {
    expect(() => decisionAction(toolCalls, { protocolRepairCount: 0, cardRepairExhausted: false }, "tool_execution"))
      .toThrow(RunGraphTransitionError)
    expect(() => toolBatchAction({ kind: "continue" }, "model"))
      .toThrow(RunGraphTransitionError)
    expect(() => resumeAction({ kind: "finalize" }, "model"))
      .toThrow(RunGraphTransitionError)
  })
})
