import type { AgentToolContract } from "../src/tools/contracts.js"

export function responseToolContract(overrides: Partial<AgentToolContract> = {}): AgentToolContract {
  return {
    allowed: true,
    resourceTypes: ["test-resource"],
    action: "read",
    sideEffect: "none",
    idempotent: true,
    replaySafe: true,
    risk: "low",
    approval: "never",
    intents: ["测试工具执行"],
    useWhen: ["测试需要执行当前工具时"],
    avoidWhen: [],
    prerequisites: [],
    parameterSummary: [],
    successEvidence: ["响应状态符合契约"],
    commonErrorCodes: [],
    predecessors: [],
    followups: [],
    verification: { mode: "response", successCodes: [200] },
    ...overrides,
  }
}
