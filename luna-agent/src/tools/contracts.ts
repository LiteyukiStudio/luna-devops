import { z } from "zod"

export const agentToolActionSchema = z.enum([
  "discover",
  "read",
  "create",
  "update",
  "delete",
  "execute",
  "verify",
])

export const agentToolSideEffectSchema = z.enum([
  "none",
  "external-read",
  "external-write",
  "platform-write",
  "destructive",
])

export const agentToolRiskSchema = z.enum(["low", "medium", "high", "critical"])
export const agentToolApprovalSchema = z.enum(["never", "always"])

const responseVerificationSchema = z.object({
  mode: z.literal("response"),
  successCodes: z.array(z.number().int().min(100).max(599)).min(1),
}).strict()

const readbackCompletionSchema = z.discriminatedUnion("mode", [
  z.object({ mode: z.literal("readback-success") }).strict(),
  z.object({
    mode: z.literal("state"),
    path: z.string().startsWith("/"),
    pendingStates: z.array(z.string()).default([]),
    successStates: z.array(z.string()).min(1),
    failureStates: z.array(z.string()).default([]),
  }).strict(),
])

const readbackVerificationSchema = z.object({
  mode: z.enum(["readback", "async-readback"]),
  operationId: z.string().min(1),
  idSource: z.string().startsWith("/"),
  argumentBindings: z.record(z.string(), z.string().startsWith("/")),
  completion: readbackCompletionSchema,
}).strict()

export const agentToolVerificationSchema = z.union([
  responseVerificationSchema,
  readbackVerificationSchema,
])

export const agentToolContractSchema = z.object({
  allowed: z.boolean(),
  resourceTypes: z.array(z.string().min(1)).min(1),
  action: agentToolActionSchema,
  sideEffect: agentToolSideEffectSchema,
  idempotent: z.boolean(),
  replaySafe: z.boolean(),
  risk: agentToolRiskSchema,
  approval: agentToolApprovalSchema,
  mfaPurpose: z.string().min(1).optional(),
  intents: z.array(z.string().min(1)).min(1),
  useWhen: z.array(z.string().min(1)).min(1),
  avoidWhen: z.array(z.string().min(1)).default([]),
  prerequisites: z.array(z.string().min(1)).default([]),
  parameterSummary: z.array(z.string().min(1)).default([]),
  successEvidence: z.array(z.string().min(1)).min(1),
  commonErrorCodes: z.array(z.string().min(1)).default([]),
  predecessors: z.array(z.string().min(1)).default([]),
  followups: z.array(z.string().min(1)).default([]),
  verification: agentToolVerificationSchema,
}).strict().superRefine((value, context) => {
  if (["external-write", "platform-write", "destructive"].includes(value.sideEffect)
    && value.avoidWhen.length === 0) {
    context.addIssue({ code: "custom", path: ["avoidWhen"], message: "ai.tool_contract_avoid_when_required" })
  }
  if (["external-write", "platform-write", "destructive"].includes(value.sideEffect)
    && value.prerequisites.length === 0) {
    context.addIssue({ code: "custom", path: ["prerequisites"], message: "ai.tool_contract_prerequisites_required" })
  }
  if (["high", "critical"].includes(value.risk) && value.approval !== "always") {
    context.addIssue({ code: "custom", path: ["approval"], message: "ai.tool_contract_approval_required" })
  }
  if (value.verification.mode !== "response" && value.sideEffect === "none") {
    context.addIssue({ code: "custom", path: ["verification"], message: "ai.tool_contract_readback_without_side_effect" })
  }
})

export type AgentToolAction = z.infer<typeof agentToolActionSchema>
export type AgentToolSideEffect = z.infer<typeof agentToolSideEffectSchema>
export type AgentToolRisk = z.infer<typeof agentToolRiskSchema>
export type AgentToolVerification = z.infer<typeof agentToolVerificationSchema>
export type AgentToolContract = z.infer<typeof agentToolContractSchema>

export type ToolRetrievalPendingState = "user_input" | "approval" | "mfa" | "async_terminal_check"

export type ToolRetrievalQuery = {
  currentGoal: string
  routeName?: string
  resourceContext: string[]
  completedOperations: string[]
  stableOutcomes: string[]
  pendingState?: ToolRetrievalPendingState
  stableErrorCodes: string[]
}

export type ToolRetrievalReason =
  | "goal_match"
  | "required_predecessor"
  | "required_verifier"
  | "sticky_operation"
  | "workflow_followup"
  | "ambiguous_candidate"

export type ToolRetrievalMatch = {
  operationId: string
  relevance: number
  reasonCode: ToolRetrievalReason
  missingPrerequisites: string[]
  ranks: Partial<Record<"intent" | "parameters" | "workflow" | "lexical", number>>
}

export type ToolRetrievalOutcome = "succeeded" | "degraded" | "unavailable"

export type ToolRetrievalResult = {
  query: ToolRetrievalQuery
  matches: ToolRetrievalMatch[]
  loadedOperationIds: string[]
  totalMatches: number
  strategy: "hybrid" | "lexical_workflow" | "lexical" | "base_only"
  outcome: ToolRetrievalOutcome
  degradedReason?: string
}

export type ToolArgumentIssue = {
  path: string
  code: string
  allowedValues?: unknown[]
  remediation?: string
}

export type ToolArgumentsInvalid = {
  code: "ai.tool_arguments_invalid"
  retryable: boolean
  issues: ToolArgumentIssue[]
}

export type ToolLoopFingerprint = {
  operationId: string
  argumentsHash: string
  stableErrorCode?: string
  stableResultHash?: string
}

export type ToolLoopStop = {
  code: "ai.tool_deterministic_failure_repeated" | "ai.tool_no_new_information" | "ai.run_tool_call_budget_exceeded"
  retryable: false
  fingerprint: ToolLoopFingerprint
}
