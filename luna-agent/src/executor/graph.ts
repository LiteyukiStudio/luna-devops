import type { RunCheckpointPhase } from "../domain.js"
import type { RunResumePlan } from "./checkpoint.js"
import type { ModelDecision } from "./decision.js"
import type { ToolBatchResult } from "./tool-step.js"

export const maxProtocolRepairs = 1

export type DecisionAction =
  | { type: "execute_tools"; calls: Extract<ModelDecision, { kind: "tool_calls" }>["calls"] }
  | { type: "finalize"; text: string; finishReason?: string }
  | { type: "repair" }
  | { type: "retry" }
  | { type: "fail"; errorCode: string }

export type DecisionContext = {
  protocolRepairCount: number
  cardRepairExhausted: boolean
}

export function decisionAction(
  decision: ModelDecision,
  context: DecisionContext,
  phase: RunCheckpointPhase = "model",
): DecisionAction {
  const validate = (action: DecisionAction) => {
    assertGraphTransition(phase, action)
    return action
  }
  if (context.cardRepairExhausted) {
    if (decision.kind === "protocol_error" && decision.code === "missing_tool_calls")
      return validate({ type: "fail", errorCode: "ai.model_protocol_violation" })
    if (decision.kind !== "final" && decision.kind !== "empty")
      return validate({ type: "fail", errorCode: "ai.interaction_card_schema_invalid" })
  }
  let action: DecisionAction
  switch (decision.kind) {
    case "tool_calls":
      action = { type: "execute_tools", calls: decision.calls }
      break
    case "final":
      action = {
        type: "finalize",
        text: decision.text,
        ...(decision.finishReason === undefined ? {} : { finishReason: decision.finishReason }),
      }
      break
    case "empty":
      action = { type: "retry" }
      break
    case "protocol_error":
      if (decision.code === "invalid_finish_reason" || context.protocolRepairCount >= maxProtocolRepairs)
        action = { type: "fail", errorCode: "ai.model_protocol_violation" }
      else
        action = { type: "repair" }
      break
  }
  return validate(action)
}

export type ToolBatchAction =
  | { type: "continue_model" }
  | { type: "waiting_approval"; toolCallId: string }
  | { type: "complete" }

export function toolBatchAction(result: ToolBatchResult, phase: RunCheckpointPhase = "tool_execution"): ToolBatchAction {
  let action: ToolBatchAction
  switch (result.kind) {
    case "continue":
      action = { type: "continue_model" }
      break
    case "waiting_approval":
      action = { type: "waiting_approval", toolCallId: result.toolCallId }
      break
    case "complete":
      action = { type: "complete" }
      break
  }
  assertGraphTransition(phase, action)
  return action
}

export type ResumeAction =
  | { type: "call_model" }
  | { type: "settle_tools" }
  | { type: "waiting_approval"; toolCallId: string }
  | { type: "finalize"; finalOutput?: string }

export type GraphAction = DecisionAction | ToolBatchAction | ResumeAction

const actionsByPhase: Record<RunCheckpointPhase, ReadonlySet<GraphAction["type"]>> = {
  model: new Set(["call_model", "execute_tools", "finalize", "repair", "retry", "fail"]),
  tool_execution: new Set(["settle_tools", "continue_model", "waiting_approval", "complete"]),
  awaiting_input: new Set(["call_model"]),
  awaiting_approval: new Set(["settle_tools", "waiting_approval"]),
  finalizing: new Set(["finalize"]),
}

export class RunGraphTransitionError extends Error {
  override readonly name = "RunGraphTransitionError"

  constructor(
    readonly phase: RunCheckpointPhase,
    readonly action: GraphAction["type"],
  ) {
    super("ai.run_graph_illegal_transition")
  }
}

export function assertGraphTransition(phase: RunCheckpointPhase, action: GraphAction): void {
  if (!actionsByPhase[phase].has(action.type)) throw new RunGraphTransitionError(phase, action.type)
}

const resumeActionsByPhase: Record<RunCheckpointPhase, ReadonlySet<ResumeAction["type"]>> = {
  model: new Set(["call_model"]),
  tool_execution: new Set(["settle_tools"]),
  awaiting_input: new Set(["call_model"]),
  awaiting_approval: new Set(["settle_tools", "waiting_approval"]),
  finalizing: new Set(["finalize"]),
}

export function resumeAction(plan: RunResumePlan, phase: RunCheckpointPhase): ResumeAction {
  let action: ResumeAction
  switch (plan.kind) {
    case "model":
      action = { type: "call_model" }
      break
    case "settle_tool_batch":
      action = { type: "settle_tools" }
      break
    case "await_approval":
      action = { type: "waiting_approval", toolCallId: plan.toolCallId }
      break
    case "finalize":
      action = plan.finalOutput === undefined
        ? { type: "finalize" }
        : { type: "finalize", finalOutput: plan.finalOutput }
      break
  }
  if (!resumeActionsByPhase[phase].has(action.type)) throw new RunGraphTransitionError(phase, action.type)
  return action
}
