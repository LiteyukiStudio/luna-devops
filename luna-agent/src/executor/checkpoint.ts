import type { RunCheckpoint, RunCheckpointState } from "../domain.js"
import type { LoopGuardState } from "../tools/loop-guard.js"

/** 一个 Run 允许的启动恢复次数；超过后不再续跑，避免崩溃-恢复循环。 */
export const maxRunRecoveries = 3

/**
 * 重启后的续跑动作。阶段、模型步数和循环保护计数只来自 checkpoint，
 * 不从 Timeline、Run 状态或进程内存推断。
 */
export type RunResumePlan =
  | { kind: "model" }
  | { kind: "await_approval"; toolCallId: string }
  /** 工具批次是否已落地只能由工具事实判断，交由 Executor 核对后决定续跑或失败。 */
  | { kind: "settle_tool_batch" }
  /** finalOutput 存在表示终态批次里还有未提交的最终答复，需要重建助手消息。 */
  | { kind: "finalize"; finalOutput?: string }

export function resumePlan(checkpoint: RunCheckpoint): RunResumePlan {
  switch (checkpoint.phase) {
    case "model":
    case "awaiting_input":
      return { kind: "model" }
    case "awaiting_approval":
      return checkpoint.state.pendingToolCallId
        ? { kind: "await_approval", toolCallId: checkpoint.state.pendingToolCallId }
        : { kind: "settle_tool_batch" }
    case "finalizing":
      return checkpoint.state.finalOutput === undefined
        ? { kind: "finalize" }
        : { kind: "finalize", finalOutput: checkpoint.state.finalOutput }
    case "tool_execution":
      return { kind: "settle_tool_batch" }
  }
}

export type CheckpointStateInput = {
  loopGuard?: LoopGuardState | undefined
  protocolRepairCount: number
  argumentRepairAttempts: ReadonlyMap<string, number>
  pendingToolCallId?: string
  finalOutput?: string
}

/** 组装一次 checkpoint 推进的状态字段；省略空值，避免写入无消费者的字段。 */
export function checkpointState(input: CheckpointStateInput): RunCheckpointState {
  return {
    ...(input.loopGuard ? { loopGuard: input.loopGuard } : {}),
    ...(input.protocolRepairCount > 0 ? { protocolRepairCount: input.protocolRepairCount } : {}),
    ...(input.argumentRepairAttempts.size
      ? { argumentRepairAttempts: Object.fromEntries(input.argumentRepairAttempts) }
      : {}),
    ...(input.pendingToolCallId ? { pendingToolCallId: input.pendingToolCallId } : {}),
    ...(input.finalOutput === undefined ? {} : { finalOutput: input.finalOutput }),
  }
}

export function restoredArgumentRepairAttempts(checkpoint: RunCheckpoint | undefined): Map<string, number> {
  return new Map(Object.entries(checkpoint?.state.argumentRepairAttempts ?? {}))
}
