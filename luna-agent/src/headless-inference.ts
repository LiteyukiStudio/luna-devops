import { z } from "zod"
import type { AIModelSnapshot, ActorContext } from "./domain.js"
import { genAIToolCallObject, genAIToolSpanAttributes } from "./genai-semconv.js"
import { createId } from "./id.js"
import type { DocsKnowledge, DocsLocale } from "./knowledge/docs-knowledge.js"
import type { Repository } from "./persistence/repository.js"
import type { RemoteConfigSource } from "./provider/config-client.js"
import type { ModelMessage, ModelProvider, ModelToolDefinition, OfficialModelUsage } from "./provider/provider.js"
import { redact, redactSensitivePaths } from "./redaction.js"
import { agentMetrics, errorDiagnostic, internalSpanOptions, recordAIContent, stableErrorCode, telemetryLog, withSpan } from "./telemetry.js"
import { ToolArgumentsInvalidError, validateToolArguments } from "./tools/argument-validator.js"
import type { ToolCatalog, ToolOperation } from "./tools/catalog.js"
import type { ToolCatalogRegistry } from "./tools/catalog-registry.js"
import { listDocsInput, listDocsTool, readDocInput, readDocTool, searchDocsInput, searchDocsTool } from "./tools/docs-tools.js"
import type { LunaApiToolClient } from "./tools/luna-api-client.js"
import { getToolDetailsInput, getToolDetailsTool } from "./tools/tool-details.js"
import { searchToolsInput, searchToolsTool } from "./tools/tool-search.js"

export const headlessTaskTypes = [
  "incident_diagnosis",
  "delivery_failure_diagnosis",
  "repo_config_infer",
] as const

export type HeadlessTaskType = typeof headlessTaskTypes[number]

export const headlessToolTiers = ["none", "L0"] as const
export type HeadlessToolTier = typeof headlessToolTiers[number]

export const headlessInferenceInputSchema = z.object({
  actorUserId: z.string().trim().min(1).max(64).optional(),
  projectId: z.string().trim().max(64).optional(),
  taskType: z.enum(headlessTaskTypes),
  contextPayload: z.record(z.string(), z.unknown()),
  maxToolTier: z.enum(headlessToolTiers).default("none"),
  maxSteps: z.number().int().min(1).max(8).optional(),
  modelId: z.string().trim().min(1).max(64).optional(),
  outputSchema: z.record(z.string(), z.unknown()).optional(),
}).strict()

export type HeadlessInferenceInput = z.infer<typeof headlessInferenceInputSchema>

export type HeadlessInferenceResult = {
  runId: string
  taskType: HeadlessTaskType
  modelId: string
  modelName: string
  output: Record<string, unknown>
  usage: {
    inputTokens: number
    outputTokens: number
    totalTokens: number
  }
}

export type HeadlessInferenceDependencies = {
  repository: Pick<Repository, "assertPositiveAvailableBalance" | "resolveLiveActorRole">
  provider: ModelProvider
  remoteConfig: Pick<RemoteConfigSource, "current">
  catalogRegistry?: Pick<ToolCatalogRegistry, "forRole">
  toolClient?: LunaApiToolClient
  docsKnowledge?: DocsKnowledge
}

const submitStructuredOutputToolName = "submit_structured_output"
const maxHeadlessOutputTokens = 4096
const maxAutoLoadedSearchTools = 4
const maxLoadedL0Tools = 12
const maxToolResultChars = 16_000

const defaultDiagnosisOutputSchema: Record<string, unknown> = {
  type: "object",
  properties: {
    rootCauseSummary: { type: "string", minLength: 1, maxLength: 600 },
    confidence: { type: "string", enum: ["high", "medium", "low"] },
    suggestions: {
      type: "array",
      items: { type: "string", minLength: 1, maxLength: 600 },
      minItems: 1,
      maxItems: 8,
    },
    evidence: {
      type: "array",
      items: { type: "string", maxLength: 500 },
      maxItems: 8,
    },
    proposedAction: {
      type: "object",
      properties: {
        toolName: { type: "string", minLength: 1, maxLength: 120 },
        arguments: { type: "object" },
        summary: { type: "string", minLength: 1, maxLength: 400 },
      },
      required: ["toolName", "arguments", "summary"],
      additionalProperties: false,
    },
  },
  required: ["rootCauseSummary", "suggestions"],
  additionalProperties: false,
}

const defaultRepoConfigOutputSchema: Record<string, unknown> = {
  type: "object",
  properties: {
    summary: { type: "string", minLength: 1, maxLength: 800 },
    confidence: { type: "string", enum: ["high", "medium", "low"] },
    buildDefinitionMode: { type: "string", enum: ["repository_dockerfile", "template"] },
    dockerfilePath: { type: "string" },
    buildContext: { type: "string" },
    buildDirectory: { type: "string" },
    buildTemplateId: { type: "string" },
    buildTemplateVersion: { type: "string" },
    buildTemplateValues: { type: "object", additionalProperties: { type: "string" } },
    buildArgs: {
      type: "array",
      items: {
        type: "object",
        properties: {
          key: { type: "string", minLength: 1 },
          value: { type: "string" },
        },
        required: ["key", "value"],
        additionalProperties: false,
      },
    },
    servicePorts: {
      type: "array",
      items: {
        type: "object",
        properties: {
          name: { type: "string" },
          port: { type: "integer", minimum: 1, maximum: 65535 },
          appProtocol: { type: "string" },
        },
        required: ["port"],
        additionalProperties: false,
      },
    },
    environmentVariables: {
      type: "array",
      items: {
        type: "object",
        properties: {
          key: { type: "string", minLength: 1 },
          secret: { type: "boolean" },
          placeholder: { type: "string" },
        },
        required: ["key", "secret"],
        additionalProperties: false,
      },
    },
    cpuRequest: { type: "string" },
    memoryRequest: { type: "string" },
    containerCommand: { type: "array", items: { type: "string" } },
    containerArgs: { type: "array", items: { type: "string" } },
    notes: { type: "array", items: { type: "string" } },
  },
  required: ["summary", "buildDefinitionMode"],
  additionalProperties: false,
}

export class HeadlessInferenceRunner {
  constructor(private readonly deps: HeadlessInferenceDependencies) {}

  async run(actor: ActorContext, rawInput: HeadlessInferenceInput, signal?: AbortSignal): Promise<HeadlessInferenceResult> {
    const input = headlessInferenceInputSchema.parse(rawInput)
    if (input.actorUserId && input.actorUserId !== actor.userId) {
      throw new Error("ai.unauthorized")
    }
    const actorProjectId = actor.projectId?.trim() || undefined
    const inputProjectId = input.projectId?.trim() || undefined
    if (actorProjectId && inputProjectId && actorProjectId !== inputProjectId) {
      throw new Error("ai.project_scope_mismatch")
    }
    const effectiveProjectId = actorProjectId || inputProjectId
    const runId = actor.runId?.trim() || createId("airun")

    return withSpan("invoke_agent luna-headless", internalSpanOptions({
      "gen_ai.operation.name": "invoke_agent",
      "gen_ai.agent.name": "luna-headless",
      "luna.run.id": runId,
      "luna.headless.task_type": input.taskType,
      "luna.headless.max_tool_tier": input.maxToolTier,
    }), async () => {
      const startedAt = performance.now()
      let runOutcome = "completed"
      agentMetrics.activeRuns.add(1)
      try {
        const liveRole = this.deps.repository.resolveLiveActorRole
          ? await this.deps.repository.resolveLiveActorRole(actor.userId)
          : (actor.platformRole ?? "user")
        if (!liveRole) {
          throw new Error("ai.unauthorized")
        }

        const config = this.deps.remoteConfig.current()
        if (!config) throw new Error("ai.provider_config_unavailable")
        if (!config.provider.configured || config.provider.models.length === 0) {
          throw new Error("ai.not_configured")
        }
        const selectedModel = selectModel(config.provider.models, input.modelId)
        if (!selectedModel) {
          throw new Error("ai.model_not_available")
        }

        const outputSchema = input.outputSchema ?? defaultOutputSchemaForTask(input.taskType)
        const submitTool: ModelToolDefinition = {
          operationId: submitStructuredOutputToolName,
          description: "提交最终结构化 JSON 结果并立即结束推理。参数必须严格满足 schema。",
          inputSchema: outputSchema,
        }

        const catalog = input.maxToolTier === "L0" && this.deps.catalogRegistry
          ? this.deps.catalogRegistry.forRole(liveRole)
          : undefined

        const maxSteps = input.maxSteps ?? (input.maxToolTier === "L0" ? 5 : 1)
        const loadedL0Ids: string[] = []
        const messages: ModelMessage[] = [
          {
            role: "system",
            content: buildSystemPrompt(input.taskType, actor.locale, input.maxToolTier, effectiveProjectId, outputSchema),
          },
          {
            role: "user",
            content: buildUserPrompt(input.taskType, input.contextPayload),
          },
        ]

        const aggregatedUsage: OfficialModelUsage = {
          inputTokens: 0,
          outputTokens: 0,
          totalTokens: 0,
        }

        let candidateOutput: unknown
        let lastValidationError: ToolArgumentsInvalidError | undefined
        let executedToolCallCount = 0
        const maxHeadlessToolCalls = 12

        for (let step = 0; step < maxSteps; step += 1) {
          await this.deps.repository.assertPositiveAvailableBalance(actor.userId)
          const isLastStep = step === maxSteps - 1
          const activeTools = buildStepTools({
            submitTool,
            maxToolTier: input.maxToolTier,
            isLastStep,
            catalog,
            loadedL0Ids,
            hasDocs: Boolean(this.deps.docsKnowledge),
          })

          const response = await this.deps.provider.complete({
            messages,
            tools: activeTools,
            toolChoice: "auto",
            maxOutputTokens: Math.min(maxHeadlessOutputTokens, selectedModel.maxOutputTokens),
            ...(signal ? { signal } : {}),
            modelId: selectedModel.id,
            modelName: selectedModel.name,
            modelPricing: selectedModel,
            budget: {
              runId,
              ownerUserId: actor.userId,
              operation: "assistant",
            },
            providerConfig: config,
          })
          accumulateUsage(aggregatedUsage, response.usage.status === "reported" ? response.usage.value : undefined)

          const toolCalls = response.toolCalls ?? []
          const submitCall = toolCalls.find(call => call.operationId === submitStructuredOutputToolName)
          if (submitCall) {
            if (submitCall.argumentError) {
              lastValidationError = new ToolArgumentsInvalidError([
                { path: "", code: "invalid_json", remediation: submitCall.argumentError.message },
              ])
            }
            else {
              try {
                const validated = validateToolArguments(outputSchema, submitCall.arguments)
                telemetryLog("agent.headless_inference.completed", "info", {
                  "operation": "agent.headless_inference",
                  "outcome": "succeeded",
                  "luna.run.id": runId,
                  "luna.headless.task_type": input.taskType,
                  "luna.headless.steps": step + 1,
                })
                return {
                  runId,
                  taskType: input.taskType,
                  modelId: selectedModel.id,
                  modelName: selectedModel.name,
                  output: validated,
                  usage: {
                    inputTokens: aggregatedUsage.inputTokens,
                    outputTokens: aggregatedUsage.outputTokens,
                    totalTokens: aggregatedUsage.totalTokens,
                  },
                }
              }
              catch (error) {
                if (error instanceof ToolArgumentsInvalidError) {
                  lastValidationError = error
                }
                else {
                  throw error
                }
              }
            }
          }

          if (!submitCall && toolCalls.length === 0) {
            const parsedText = tryParseJsonObject(response.text)
            if (parsedText !== undefined) {
              try {
                const validated = validateToolArguments(outputSchema, parsedText)
                telemetryLog("agent.headless_inference.completed", "info", {
                  "operation": "agent.headless_inference",
                  "outcome": "succeeded",
                  "luna.run.id": runId,
                  "luna.headless.task_type": input.taskType,
                  "luna.headless.steps": step + 1,
                })
                return {
                  runId,
                  taskType: input.taskType,
                  modelId: selectedModel.id,
                  modelName: selectedModel.name,
                  output: validated,
                  usage: {
                    inputTokens: aggregatedUsage.inputTokens,
                    outputTokens: aggregatedUsage.outputTokens,
                    totalTokens: aggregatedUsage.totalTokens,
                  },
                }
              }
              catch (error) {
                if (error instanceof ToolArgumentsInvalidError) {
                  lastValidationError = error
                }
                else {
                  throw error
                }
              }
            }
            candidateOutput = response.text
            break
          }

          if (submitCall && lastValidationError) {
            candidateOutput = submitCall.arguments
            break
          }

          if (input.maxToolTier !== "L0" || isLastStep) {
            candidateOutput = response.text
            break
          }

          const normalizedCalls = toolCalls.map((call) => {
            const sensitivePaths = catalog?.getDetails([call.operationId]).items[0]?.sensitivePaths ?? []
            return {
              ...call,
              id: call.id || createId("aitc"),
              arguments: sensitivePaths.length > 0
                ? redactSensitivePaths(call.arguments, sensitivePaths)
                : call.arguments,
            }
          })
          messages.push({
            role: "assistant",
            content: response.text || "",
            toolCalls: normalizedCalls,
            ...(response.reasoningSummary ? { reasoning: response.reasoningSummary } : {}),
          })

          for (let idx = 0; idx < toolCalls.length; idx += 1) {
            const rawCall = toolCalls[idx]!
            const call = normalizedCalls[idx]!
            executedToolCallCount += 1
            if (executedToolCallCount > maxHeadlessToolCalls) {
              messages.push({
                role: "tool",
                toolCallId: call.id,
                content: JSON.stringify({ error: "ai.tool_call_limit_exceeded", message: "单次无头诊断工具调用次数已达上限，请直接提交当前结论。" }),
              })
              continue
            }
            const toolOutput = await this.executeHeadlessToolCall({
              runId,
              actorUserId: actor.userId,
              projectId: effectiveProjectId,
              locale: actor.locale,
              catalog,
              loadedL0Ids,
              callId: call.id,
              operationId: call.operationId,
              rawArguments: rawCall.arguments,
              argumentError: rawCall.argumentError?.message,
              signal,
            })
            const sensitivePaths = catalog?.getDetails([call.operationId]).items[0]?.sensitivePaths ?? []
            const safeToolOutput = sensitivePaths.length > 0
              ? redactSensitivePaths(toolOutput, sensitivePaths)
              : toolOutput
            messages.push({
              role: "tool",
              toolCallId: call.id,
              content: truncateToolOutput(JSON.stringify(safeToolOutput)),
            })
          }
        }

        // Single bounded repair attempt when the model stopped with text or invalid schema arguments.
        await this.deps.repository.assertPositiveAvailableBalance(actor.userId)
        const draftPreview = typeof candidateOutput === "string"
          ? candidateOutput
          : candidateOutput !== undefined
            ? JSON.stringify(candidateOutput)
            : ""
        const repairPrompt = lastValidationError
          ? `上一步输出未通过 JSON Schema 校验：${JSON.stringify(lastValidationError.issues)}。请修正后立即调用 ${submitStructuredOutputToolName} 提交完整合规对象。`
          : `请立即调用 ${submitStructuredOutputToolName}（或仅输出纯 JSON 对象）提交最终结构化结果。此前草稿：${draftPreview.slice(0, 2000)}`
        messages.push({ role: "user", content: repairPrompt })

        const repairResponse = await this.deps.provider.complete({
          messages,
          tools: [submitTool],
          toolChoice: "auto",
          maxOutputTokens: Math.min(maxHeadlessOutputTokens, selectedModel.maxOutputTokens),
          ...(signal ? { signal } : {}),
          modelId: selectedModel.id,
          modelName: selectedModel.name,
          modelPricing: selectedModel,
          budget: {
            runId,
            ownerUserId: actor.userId,
            operation: "assistant",
          },
          providerConfig: config,
        })
        accumulateUsage(aggregatedUsage, repairResponse.usage.status === "reported" ? repairResponse.usage.value : undefined)

        const repairSubmit = (repairResponse.toolCalls ?? []).find(call => call.operationId === submitStructuredOutputToolName)
        const finalRaw = repairSubmit && !repairSubmit.argumentError
          ? repairSubmit.arguments
          : tryParseJsonObject(repairResponse.text)
        if (finalRaw === undefined) {
          throw new Error("ai.structured_output_invalid")
        }
        let validated: Record<string, unknown>
        try {
          validated = validateToolArguments(outputSchema, finalRaw)
        }
        catch (error) {
          if (error instanceof ToolArgumentsInvalidError) {
            throw new Error("ai.structured_output_invalid")
          }
          throw error
        }

        telemetryLog("agent.headless_inference.completed", "info", {
          "operation": "agent.headless_inference",
          "outcome": "succeeded",
          "luna.run.id": runId,
          "luna.headless.task_type": input.taskType,
          "luna.headless.repaired": true,
        })
        return {
          runId,
          taskType: input.taskType,
          modelId: selectedModel.id,
          modelName: selectedModel.name,
          output: validated,
          usage: {
            inputTokens: aggregatedUsage.inputTokens,
            outputTokens: aggregatedUsage.outputTokens,
            totalTokens: aggregatedUsage.totalTokens,
          },
        }
      }
      catch (error) {
        const rawNormalized = error instanceof Error ? error : new Error(String(error))
        const normalized = rawNormalized.message === "ai.provider_context_length_exceeded"
          ? new Error("ai.model_context_insufficient")
          : rawNormalized
        const isCanceled = normalized.name === "AbortError" || normalized.message === "ai.run_canceled"
        runOutcome = isCanceled ? "canceled" : stableErrorCode(normalized)
        telemetryLog("agent.headless_inference.failed", isCanceled ? "info" : "warn", {
          "operation": "agent.headless_inference",
          "outcome": isCanceled ? "cancelled" : "failed",
          "luna.run.id": runId,
          "luna.headless.task_type": input.taskType,
          ...errorDiagnostic(normalized, runOutcome),
        })
        throw normalized
      }
      finally {
        const metricAttributes = { outcome: runOutcome }
        agentMetrics.activeRuns.add(-1)
        agentMetrics.runs.add(1, metricAttributes)
        agentMetrics.runDuration.record((performance.now() - startedAt) / 1000, metricAttributes)
      }
    })
  }

  private async executeHeadlessToolCall(input: {
    runId: string
    actorUserId: string
    projectId: string | undefined
    locale: string
    catalog: ToolCatalog | undefined
    loadedL0Ids: string[]
    callId: string
    operationId: string
    rawArguments: Record<string, unknown>
    argumentError: string | undefined
    signal: AbortSignal | undefined
  }): Promise<unknown> {
    const startedAt = performance.now()
    let outcome = "succeeded"
    const catalogOp = input.catalog?.getDetails([input.operationId]).items[0]
    const isBuiltInHeadlessTool = input.operationId === "search_tools"
      || input.operationId === "get_tool_details"
      || input.operationId === "search_docs"
      || input.operationId === "list_docs"
      || input.operationId === "read_doc"
    const telemetryToolName = isBuiltInHeadlessTool || Boolean(catalogOp) ? input.operationId : "unknown"
    const sensitivePaths = catalogOp?.sensitivePaths ?? []
    const safeArgsForTrace = sensitivePaths.length > 0
      ? redactSensitivePaths(input.rawArguments, sensitivePaths)
      : input.rawArguments

    try {
      return await withSpan(`execute_tool ${telemetryToolName}`, internalSpanOptions({
        ...genAIToolSpanAttributes({ name: telemetryToolName, callId: input.callId }),
        "luna.run.id": input.runId,
      }), async (span) => {
        recordAIContent(span, "luna.gen_ai.tool.content.input", "gen_ai.tool.call.arguments", genAIToolCallObject(safeArgsForTrace), {
          "gen_ai.tool.name": telemetryToolName,
        })
        let result: unknown
        try {
          result = await this.executeHeadlessToolCallInner(input)
        }
        catch (error) {
          outcome = stableErrorCode(error)
          recordAIContent(span, "luna.gen_ai.tool.content.output", "gen_ai.tool.call.result", {
            error: {
              type: error instanceof Error ? error.name : "UnknownError",
              code: outcome,
            },
          }, {
            "gen_ai.tool.name": telemetryToolName,
          })
          throw error
        }
        if (result && typeof result === "object" && "error" in result && typeof (result as { error?: unknown }).error === "string") {
          outcome = (result as { error: string }).error
        }
        const safeResultForTrace = sensitivePaths.length > 0
          ? redactSensitivePaths(result, sensitivePaths)
          : result
        recordAIContent(span, "luna.gen_ai.tool.content.output", "gen_ai.tool.call.result", genAIToolCallObject(safeResultForTrace), {
          "gen_ai.tool.name": telemetryToolName,
        })
        return result
      })
    }
    finally {
      const attributes = { tool: telemetryToolName, outcome }
      agentMetrics.toolCalls.add(1, attributes)
      agentMetrics.toolDuration.record((performance.now() - startedAt) / 1000, attributes)
    }
  }

  private async executeHeadlessToolCallInner(input: {
    runId: string
    actorUserId: string
    projectId: string | undefined
    locale: string
    catalog: ToolCatalog | undefined
    loadedL0Ids: string[]
    callId: string
    operationId: string
    rawArguments: Record<string, unknown>
    argumentError: string | undefined
    signal: AbortSignal | undefined
  }): Promise<unknown> {
    if (input.argumentError) {
      return { error: "invalid_json", message: input.argumentError }
    }
    if (input.operationId === "search_tools") {
      if (!input.catalog) return { items: [], total: 0 }
      const parsed = searchToolsInput.safeParse(input.rawArguments)
      if (!parsed.success) return { error: "invalid_arguments", message: parsed.error.message }
      const page = input.catalog.search(parsed.data)
      for (const item of page.items.slice(0, maxAutoLoadedSearchTools)) {
        const op = input.catalog.getDetails([item.operationId]).items[0]
        if (op && isExecutableHeadlessL0Operation(op)) {
          rememberBounded(input.loadedL0Ids, op.operationId, maxLoadedL0Tools)
        }
      }
      return page
    }
    if (input.operationId === "get_tool_details") {
      if (!input.catalog) return { items: [], missingOperationIds: [] }
      const parsed = getToolDetailsInput.safeParse(input.rawArguments)
      if (!parsed.success) return { error: "invalid_arguments", message: parsed.error.message }
      const details = input.catalog.semanticDetails(parsed.data.operationIds)
      const fullOps = input.catalog.getDetails(parsed.data.operationIds).items
      for (const op of fullOps) {
        if (isExecutableHeadlessL0Operation(op)) {
          rememberBounded(input.loadedL0Ids, op.operationId, maxLoadedL0Tools)
        }
      }
      // Include inputSchema on non-L0 operations so the model can construct accurate proposedAction.arguments!
      const enriched = details.items.map((semantic) => {
        const full = fullOps.find(item => item.operationId === semantic.operationId)
        return full ? { ...semantic, inputSchema: full.inputSchema } : semantic
      })
      return { items: enriched, missingOperationIds: details.missingOperationIds }
    }
    if (input.operationId === "search_docs" && this.deps.docsKnowledge) {
      const parsed = searchDocsInput.safeParse(input.rawArguments)
      if (!parsed.success) return { error: "invalid_arguments", message: parsed.error.message }
      const locale: DocsLocale = parsed.data.locale ?? (input.locale.toLowerCase().startsWith("zh") ? "zh" : "en")
      return { items: this.deps.docsKnowledge.search(parsed.data.query, locale, parsed.data.limit) }
    }
    if (input.operationId === "list_docs" && this.deps.docsKnowledge) {
      const parsed = listDocsInput.safeParse(input.rawArguments)
      if (!parsed.success) return { error: "invalid_arguments", message: parsed.error.message }
      return this.deps.docsKnowledge.list(parsed.data.locale, parsed.data.path, parsed.data.depth)
    }
    if (input.operationId === "read_doc" && this.deps.docsKnowledge) {
      const parsed = readDocInput.safeParse(input.rawArguments)
      if (!parsed.success) return { error: "invalid_arguments", message: parsed.error.message }
      return this.deps.docsKnowledge.read(parsed.data.docId, parsed.data.sectionId) ?? { error: "doc_not_found" }
    }

    const op = input.catalog?.getDetails([input.operationId]).items[0]
    if (!op) {
      return { error: "ai.tool_not_available", message: `未知工具 ${input.operationId}` }
    }
    if (!isExecutableHeadlessL0Operation(op)) {
      return {
        error: "ai.headless_tier_exceeded",
        message: `当前无头诊断仅允许直接执行 L0 只读工具。${op.operationId} 属于写操作/高风险等级，请勿直接调用，而是将其填入最终输出的 proposedAction 字段供系统或用户审批执行。`,
      }
    }
    if (!this.deps.toolClient?.executeHeadlessL0) {
      return { error: "ai.tool_client_unavailable", message: "L0 工具执行客户端未配置" }
    }

    let validatedArgs: Record<string, unknown>
    try {
      validatedArgs = validateToolArguments(op.inputSchema, input.rawArguments)
    }
    catch (error) {
      if (error instanceof ToolArgumentsInvalidError) {
        return error.toJSON()
      }
      throw error
    }

    const result = await this.deps.toolClient.executeHeadlessL0({
      runId: input.runId,
      toolCallId: input.callId,
      actorUserId: input.actorUserId,
      ...(input.projectId ? { projectId: input.projectId } : {}),
      operation: op,
      arguments: validatedArgs,
      ...(input.signal ? { signal: input.signal } : {}),
    })
    return { status: result.status, data: result.body }
  }
}

function selectModel(models: readonly AIModelSnapshot[], requestedModelId?: string): AIModelSnapshot | undefined {
  if (!requestedModelId) return models[0]
  return models.find(model => model.id === requestedModelId)
}

function defaultOutputSchemaForTask(taskType: HeadlessTaskType): Record<string, unknown> {
  if (taskType === "repo_config_infer") return defaultRepoConfigOutputSchema
  return defaultDiagnosisOutputSchema
}

function isExecutableHeadlessL0Operation(op: ToolOperation): boolean {
  if (op.verificationOnly) return false
  const tier = op.approvalTier ?? (op.requiresApproval ? "L2" : "L0")
  return tier === "L0" && op.effect === "read"
}

function rememberBounded(list: string[], value: string, max: number): void {
  const existingIdx = list.indexOf(value)
  if (existingIdx >= 0) list.splice(existingIdx, 1)
  list.push(value)
  while (list.length > max) list.shift()
}

function buildStepTools(input: {
  submitTool: ModelToolDefinition
  maxToolTier: HeadlessToolTier
  isLastStep: boolean
  catalog: ToolCatalog | undefined
  loadedL0Ids: string[]
  hasDocs: boolean
}): ModelToolDefinition[] {
  if (input.maxToolTier !== "L0" || input.isLastStep) {
    return [input.submitTool]
  }
  const tools: ModelToolDefinition[] = [input.submitTool]
  if (input.catalog) {
    tools.push(searchToolsTool, getToolDetailsTool, ...input.catalog.modelTools(input.loadedL0Ids))
  }
  if (input.hasDocs) {
    tools.push(searchDocsTool, listDocsTool, readDocTool)
  }
  return tools
}

function accumulateUsage(target: OfficialModelUsage, delta?: OfficialModelUsage): void {
  if (!delta) return
  target.inputTokens += delta.inputTokens
  target.outputTokens += delta.outputTokens
  target.totalTokens += delta.totalTokens
}

function truncateToolOutput(raw: string): string {
  const clean = redact(raw)
  if (clean.length <= maxToolResultChars) return clean
  return `${clean.slice(0, maxToolResultChars)}\n...[truncated]`
}

function tryParseJsonObject(rawText: string): Record<string, unknown> | undefined {
  const trimmed = rawText.trim()
  if (!trimmed) return undefined
  const fencedMatch = trimmed.match(/```(?:json)?\s*([\s\S]*?)```/i)
  const candidate = (fencedMatch?.[1] ?? trimmed).trim()
  const tryParse = (str: string): Record<string, unknown> | undefined => {
    try {
      const parsed: unknown = JSON.parse(str)
      if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
        return parsed as Record<string, unknown>
      }
    }
    catch {
      return undefined
    }
    return undefined
  }
  const direct = tryParse(candidate)
  if (direct) return direct
  const firstBrace = candidate.indexOf("{")
  const lastBrace = candidate.lastIndexOf("}")
  if (firstBrace >= 0 && lastBrace > firstBrace) {
    return tryParse(candidate.slice(firstBrace, lastBrace + 1))
  }
  return undefined
}

function buildSystemPrompt(
  taskType: HeadlessTaskType,
  locale: string,
  maxToolTier: HeadlessToolTier,
  projectId: string | undefined,
  outputSchema: Record<string, unknown>,
): string {
  const languageDirective = `所有面向人类阅读的自然语言字段（如 summary、rootCauseSummary、suggestions、evidence、notes、proposedAction.summary）必须使用与调用方语言（${locale || "zh-CN"}）一致的语言撰写；标识符、工具名、配置键名、端口与资源单位保持原样。`
  const projectScopeDirective = projectId
    ? `当前任务严格限定在项目空间 ID "${projectId}" 内。`
    : ""
  const toolDirective = maxToolTier === "L0"
    ? "你可以按需调用 search_tools / get_tool_details 检索平台工具目录、调用只读 L0 工具核实实时状态，或通过 get_tool_details 查看修复工具的精确参数 Schema 并填入最终输出的 proposedAction（切勿直接执行非 L0 工具）。"
    : "请完全基于给定的结构化上下文进行分析，无需调用外部查询工具。"

  const taskSpecific = taskType === "repo_config_infer"
    ? [
        "你是 Luna DevOps 应用交付配置推断引擎。",
        "请根据代码仓库检测到的目录结构、Dockerfile、构建清单（package.json / go.mod / pyproject.toml / Cargo.toml / pom.xml 等）和可用构建模板列表（availableTemplates），推断最稳妥的部署目标配置：",
        "1) 若仓库已有可用 Dockerfile，优先使用 buildDefinitionMode=\"repository_dockerfile\" 并给出准确的 dockerfilePath、buildContext、buildDirectory 与 EXPOSE 对应的 servicePorts；",
        "2) 若仓库没有 Dockerfile，从 availableTemplates 中选择匹配的 buildTemplateId、buildTemplateVersion 并只填写该模板声明的参数键（buildTemplateValues）；",
        "3) 对可能需要注入的环境变量只输出键名（key）、是否为敏感凭据（secret）和格式占位说明（placeholder），绝不编造或回填真实密钥值；",
        "4) 给出合理的 Kubernetes 资源配额（如 cpuRequest: \"250m\" / \"500m\" / \"1\"，memoryRequest: \"256Mi\" / \"512Mi\" / \"1Gi\"）；仅当仓库明确需要覆盖镜像默认入口时才填写 containerCommand / containerArgs。",
      ].join("\n")
    : taskType === "delivery_failure_diagnosis"
      ? [
          "你是 Luna DevOps 构建与发布失败“临终诊断”专家。",
          "请结合失败阶段、稳定错误码（failureCode）、部署配置快照与末尾日志，用一句话直接点明真实根因（rootCauseSummary），给出可立即落地的修复步骤（suggestions），并在可通过单一平台工具修复（例如更新端口/资源配额、重新触发构建或回滚发布）时提供精确的 proposedAction。",
        ].join("\n")
      : [
          "你是 Luna DevOps 运行时故障巡检与分级自愈诊断专家。",
          "请结合异常类型（OOMKilled / CrashLoopBackOff / 启动或拉取失败 / 副本降级）、Pod 状态、事件与近期日志定位根因（rootCauseSummary），给出修复建议（suggestions）。",
          "当存在安全明确的单步恢复操作（例如重启异常目标 delivery_restart_target、回滚到上一稳定版本 delivery_rollback_release、或上调内存配额 delivery_update_target_runtime）时，先通过 get_tool_details 核对工具精确参数，再填入 proposedAction；若根因属于代码逻辑缺陷或外部依赖宕机且重启/扩容无效，则省略 proposedAction 避免无效自愈震荡。",
        ].join("\n")

  return [
    taskSpecific,
    projectScopeDirective,
    toolDirective,
    languageDirective,
    `完成分析后，必须调用工具 ${submitStructuredOutputToolName} 提交严格符合以下 JSON Schema 的结果对象：\n${JSON.stringify(outputSchema)}`,
  ].filter(Boolean).join("\n\n")
}

function buildUserPrompt(taskType: HeadlessTaskType, contextPayload: Record<string, unknown>): string {
  return `任务类型：${taskType}\n上下文数据（JSON）：\n${redact(JSON.stringify(contextPayload, null, 2))}`
}
