import { createHash } from "node:crypto"
import { execFile } from "node:child_process"
import { readFile } from "node:fs/promises"
import { ContextCompiler, type ContextCompilerOptions } from "../../src/context/compiler.js"
import type { AIModelSnapshot, ConversationHistoryEntry, ConversationSummary } from "../../src/domain.js"
import { ModelRuntime, type AssistantModelInput } from "../../src/model-runtime.js"
import type { Repository } from "../../src/persistence/repository.js"
import {
  OpenAIChatCompletionsProvider,
  type OpenAIChatCompletionsOptions,
} from "../../src/provider/openai-chat-completions.js"
import type {
  ModelCapabilities,
  ModelEvent,
  ModelProvider,
  ModelRequest,
  ModelResponse,
  ModelToolDefinition,
} from "../../src/provider/provider.js"

export const promptCacheBenchmarkSchemaVersion = "luna.agent.prompt-cache-benchmark.v3" as const

export type PromptCacheBenchmarkCacheEpochTransition = "within_epoch" | "cache_epoch_invalidation"

export type PromptCacheBenchmarkAssertion = {
  id: string
  description: string
  passed: boolean
}

export type PromptCacheBenchmarkStep = {
  id: string
  label: string
  requestBytes: number
  requestSha256: string
  messageCount: number
  toolCount: number
  conversationCompacted: boolean | null
}

export type PromptCacheBenchmarkTransition = {
  fromStepId: string
  toStepId: string
  commonPrefixBytes: number
  estimatedCommonPrefixTokens: number
  nextRequestReuseRatio: number
  uncachedSuffixBytes: number
  cacheEpochTransition: PromptCacheBenchmarkCacheEpochTransition
}

export type PromptCacheBenchmarkScenario = {
  id: string
  title: string
  description: string
  steps: PromptCacheBenchmarkStep[]
  transitions: PromptCacheBenchmarkTransition[]
  assertions: PromptCacheBenchmarkAssertion[]
}

export type PromptCacheBenchmarkResult = {
  schemaVersion: typeof promptCacheBenchmarkSchemaVersion
  benchmark: {
    name: "agent-prompt-cache-prefix"
    version: 3
    checkoutLabel: string
    sourceRevision: string
    implementationDigest: string
    harnessDigest: string
    serialization: "openai-compatible-chat-completions-stream-json"
    prefixMetric: "utf8-longest-common-prefix-bytes"
    tokenEstimate: "ceil(commonPrefixBytes/4)"
    providerMeasurement: false
    disclaimer: string
  }
  summary: {
    scenarioCount: number
    transitionCount: number
    assertionCount: number
    passedAssertionCount: number
    failedAssertionCount: number
    commonPrefixBytes: number
    estimatedCommonPrefixTokens: number
    nextRequestBytes: number
    uncachedSuffixBytes: number
    cacheEpochInvalidationTransitionCount: number
    weightedNextRequestReuseRatio: number
  }
  scenarios: PromptCacheBenchmarkScenario[]
}

export type RunPromptCacheBenchmarkOptions = {
  checkoutLabel?: string
  sourceRevision?: string
}

type MeasuredStep = {
  result: PromptCacheBenchmarkStep
  request: ModelRequest
  serialized: string
}

const disclaimer = "该指标只比较确定性 fixture 生成的 OpenAI-compatible 流式请求 JSON 的 UTF-8 最长公共前缀；Token 为每 4 字节的粗略估算，不是 Provider 实际缓存命中、计费或 usage。"

const implementationSourcePaths = [
  "src/context/compiler.ts",
  "src/context/model-messages.ts",
  "src/model-history.ts",
  "src/model-runtime.ts",
  "src/persistence/postgres.ts",
  "src/prompt/system.ts",
  "src/provider/openai-chat-completions.ts",
  "src/provider/managed.ts",
  "src/redaction.ts",
  "src/runtime-settings.ts",
  "src/tools/internal-operation-ids.ts",
] as const

const harnessSourcePaths = [
  "benchmarks/prompt-cache/README.md",
  "benchmarks/prompt-cache/benchmark.ts",
  "benchmarks/prompt-cache/cli.ts",
  "benchmarks/prompt-cache/comparison.ts",
  "benchmarks/prompt-cache/report.ts",
] as const

const packageRoot = new URL("../../", import.meta.url)

const model: AIModelSnapshot = {
  id: "aimdl_prompt_cache_benchmark",
  name: "prompt-cache-benchmark-model",
  maxContextTokens: 4_096,
  maxOutputTokens: 256,
  inputCreditsPerMillion: "1",
  outputCreditsPerMillion: "2",
  cachedInputCreditsPerMillion: "0.5",
}

const inspectApplicationTool: ModelToolDefinition = {
  operationId: "inspectApplication",
  description: "读取指定应用的权威当前状态；不会修改资源。",
  inputSchema: {
    type: "object",
    properties: { applicationId: { type: "string", description: "应用 ID" } },
    required: ["applicationId"],
    additionalProperties: false,
  },
}

const readDeploymentLogsTool: ModelToolDefinition = {
  operationId: "readDeploymentLogs",
  description: "读取指定部署的有界日志片段；不会修改资源。",
  inputSchema: {
    type: "object",
    properties: {
      deploymentId: { type: "string", description: "部署 ID" },
      tailLines: { type: "integer", minimum: 1, maximum: 500 },
    },
    required: ["deploymentId"],
    additionalProperties: false,
  },
}

const restartDeploymentTool: ModelToolDefinition = {
  operationId: "restartDeployment",
  description: "重启指定部署；执行后必须回读权威状态。",
  inputSchema: {
    type: "object",
    properties: { deploymentId: { type: "string", description: "部署 ID" } },
    required: ["deploymentId"],
    additionalProperties: false,
  },
}

const summaryContent = {
  userGoals: ["保持应用 alpha 可用", "完成部署状态核对"],
  constraints: ["仅使用只读操作，未经确认不得重启"],
  confirmedResources: [{ type: "application", name: "alpha", id: "app_alpha" }],
  completedActions: ["已确认应用 alpha 位于项目空间 benchmark"],
  failures: [],
  pendingWork: ["核对最新部署状态"],
  durableFacts: ["应用 alpha 的期望副本数为 3"],
}

const compilerOptions: ContextCompilerOptions = {
  compressionTriggerRatio: 0.8,
  recentTurnCount: 1,
  maxUncompressedTurnCount: 100,
  maxCompressionTurnsPerCompile: 8,
  summaryMaxOutputTokens: 512,
  maxHistoryPayloadBytes: 64 * 1_024,
  maxSummaryPayloadBytes: 8 * 1_024,
  maxContinuationPayloadBytes: 32 * 1_024,
}

/**
 * 运行不访问网络、时间或随机源的固定 Agent 上下文场景。
 * 结果可用于 checkout 间的相对比较，但不能替代 Provider 官方 usage。
 */
export async function runPromptCacheBenchmark(
  options: RunPromptCacheBenchmarkOptions = {},
): Promise<PromptCacheBenchmarkResult> {
  const [sourceRevision, implementationDigest, harnessDigest, scenarios] = await Promise.all([
    options.sourceRevision === undefined
      ? readSourceRevision()
      : Promise.resolve(normalizeSourceRevision(options.sourceRevision)),
    digestSourceFiles(implementationSourcePaths, true),
    digestSourceFiles(harnessSourcePaths),
    Promise.all([
      sameRunMultiStepScenario(),
      crossTurnHistoryScenario(),
      toolTouchAndAdditionScenario(),
      compactionScenario(),
    ]),
  ])
  return {
    schemaVersion: promptCacheBenchmarkSchemaVersion,
    benchmark: {
      name: "agent-prompt-cache-prefix",
      version: 3,
      checkoutLabel: options.checkoutLabel?.trim() || "current-checkout",
      sourceRevision,
      implementationDigest,
      harnessDigest,
      serialization: "openai-compatible-chat-completions-stream-json",
      prefixMetric: "utf8-longest-common-prefix-bytes",
      tokenEstimate: "ceil(commonPrefixBytes/4)",
      providerMeasurement: false,
      disclaimer,
    },
    summary: summarizeScenarios(scenarios),
    scenarios,
  }
}

export function failedPromptCacheBenchmarkAssertions(
  result: PromptCacheBenchmarkResult,
): Array<{ scenarioId: string, assertion: PromptCacheBenchmarkAssertion }> {
  return result.scenarios.flatMap(scenario => scenario.assertions
    .filter(assertion => !assertion.passed)
    .map(assertion => ({ scenarioId: scenario.id, assertion })))
}

export function isPromptCacheBenchmarkResult(value: unknown): value is PromptCacheBenchmarkResult {
  if (!isRecord(value)
    || !hasExactKeys(value, ["schemaVersion", "benchmark", "summary", "scenarios"])
    || value.schemaVersion !== promptCacheBenchmarkSchemaVersion) return false
  if (!isRecord(value.benchmark)
    || !hasExactKeys(value.benchmark, [
      "name",
      "version",
      "checkoutLabel",
      "sourceRevision",
      "implementationDigest",
      "harnessDigest",
      "serialization",
      "prefixMetric",
      "tokenEstimate",
      "providerMeasurement",
      "disclaimer",
    ])
    || value.benchmark.name !== "agent-prompt-cache-prefix"
    || value.benchmark.version !== 3
    || value.benchmark.providerMeasurement !== false
    || !isNonEmptyString(value.benchmark.checkoutLabel)
    || !isSourceRevision(value.benchmark.sourceRevision)
    || !isSha256(value.benchmark.implementationDigest)
    || !isSha256(value.benchmark.harnessDigest)
    || value.benchmark.serialization !== "openai-compatible-chat-completions-stream-json"
    || value.benchmark.prefixMetric !== "utf8-longest-common-prefix-bytes"
    || value.benchmark.tokenEstimate !== "ceil(commonPrefixBytes/4)"
    || typeof value.benchmark.disclaimer !== "string") return false
  if (!isRecord(value.summary)
    || !hasExactKeys(value.summary, [
      "scenarioCount",
      "transitionCount",
      "assertionCount",
      "passedAssertionCount",
      "failedAssertionCount",
      "commonPrefixBytes",
      "estimatedCommonPrefixTokens",
      "nextRequestBytes",
      "uncachedSuffixBytes",
      "cacheEpochInvalidationTransitionCount",
      "weightedNextRequestReuseRatio",
    ])
    || !Array.isArray(value.scenarios)) return false
  if (!hasUniqueStrings(value.scenarios, scenario => isRecord(scenario) ? scenario.id : undefined)) return false
  if (!value.scenarios.every(isPromptCacheBenchmarkScenario)) return false
  const result = value as PromptCacheBenchmarkResult
  return sameSummary(result.summary, summarizeScenarios(result.scenarios))
}

function isPromptCacheBenchmarkScenario(value: unknown): value is PromptCacheBenchmarkScenario {
  if (!isRecord(value)
    || !hasExactKeys(value, ["id", "title", "description", "steps", "transitions", "assertions"])
    || !isNonEmptyString(value.id)
    || !isNonEmptyString(value.title)
    || !isNonEmptyString(value.description)
    || !Array.isArray(value.steps)
    || value.steps.length < 2
    || !hasUniqueStrings(value.steps, step => isRecord(step) ? step.id : undefined)
    || !value.steps.every(isPromptCacheBenchmarkStep)
    || !Array.isArray(value.transitions)
    || value.transitions.length !== value.steps.length - 1
    || !Array.isArray(value.assertions)
    || value.assertions.length < 1
    || !hasUniqueStrings(value.assertions, assertion => isRecord(assertion) ? assertion.id : undefined)
    || !value.assertions.every(isPromptCacheBenchmarkAssertion)) return false

  const steps = value.steps
  return value.transitions.every((candidate, index) => {
    if (!isRecord(candidate)
      || !hasExactKeys(candidate, [
        "fromStepId",
        "toStepId",
        "commonPrefixBytes",
        "estimatedCommonPrefixTokens",
        "nextRequestReuseRatio",
        "uncachedSuffixBytes",
        "cacheEpochTransition",
      ])) return false
    const previous = steps[index]
    const next = steps[index + 1]
    if (!previous || !next) return false
    return candidate.fromStepId === previous.id
      && candidate.toStepId === next.id
      && isNonNegativeInteger(candidate.commonPrefixBytes)
      && candidate.commonPrefixBytes <= Math.min(previous.requestBytes, next.requestBytes)
      && candidate.estimatedCommonPrefixTokens === estimateTokens(candidate.commonPrefixBytes)
      && candidate.nextRequestReuseRatio === ratio(candidate.commonPrefixBytes, next.requestBytes)
      && candidate.uncachedSuffixBytes === next.requestBytes - candidate.commonPrefixBytes
      && isCacheEpochTransition(candidate.cacheEpochTransition)
  })
}

function isPromptCacheBenchmarkStep(value: unknown): value is PromptCacheBenchmarkStep {
  return isRecord(value)
    && hasExactKeys(value, [
      "id",
      "label",
      "requestBytes",
      "requestSha256",
      "messageCount",
      "toolCount",
      "conversationCompacted",
    ])
    && isNonEmptyString(value.id)
    && isNonEmptyString(value.label)
    && isPositiveInteger(value.requestBytes)
    && isSha256(value.requestSha256)
    && isNonNegativeInteger(value.messageCount)
    && isNonNegativeInteger(value.toolCount)
    && (typeof value.conversationCompacted === "boolean" || value.conversationCompacted === null)
}

function isPromptCacheBenchmarkAssertion(value: unknown): value is PromptCacheBenchmarkAssertion {
  return isRecord(value)
    && hasExactKeys(value, ["id", "description", "passed"])
    && isNonEmptyString(value.id)
    && isNonEmptyString(value.description)
    && typeof value.passed === "boolean"
}

function summarizeScenarios(
  scenarios: PromptCacheBenchmarkScenario[],
): PromptCacheBenchmarkResult["summary"] {
  const transitions = scenarios.flatMap(scenario => scenario.transitions)
  const assertions = scenarios.flatMap(scenario => scenario.assertions)
  const commonPrefixBytes = transitions.reduce((total, transition) => total + transition.commonPrefixBytes, 0)
  const nextRequestBytes = scenarios.reduce((total, scenario) => total
    + scenario.steps.slice(1).reduce((scenarioTotal, step) => scenarioTotal + step.requestBytes, 0), 0)
  const uncachedSuffixBytes = transitions.reduce(
    (total, transition) => total + transition.uncachedSuffixBytes,
    0,
  )
  return {
    scenarioCount: scenarios.length,
    transitionCount: transitions.length,
    assertionCount: assertions.length,
    passedAssertionCount: assertions.filter(assertion => assertion.passed).length,
    failedAssertionCount: assertions.filter(assertion => !assertion.passed).length,
    commonPrefixBytes,
    estimatedCommonPrefixTokens: estimateTokens(commonPrefixBytes),
    nextRequestBytes,
    uncachedSuffixBytes,
    cacheEpochInvalidationTransitionCount: transitions.filter(
      transition => transition.cacheEpochTransition === "cache_epoch_invalidation",
    ).length,
    weightedNextRequestReuseRatio: ratio(commonPrefixBytes, nextRequestBytes),
  }
}

function sameSummary(
  left: PromptCacheBenchmarkResult["summary"],
  right: PromptCacheBenchmarkResult["summary"],
): boolean {
  return left.scenarioCount === right.scenarioCount
    && left.transitionCount === right.transitionCount
    && left.assertionCount === right.assertionCount
    && left.passedAssertionCount === right.passedAssertionCount
    && left.failedAssertionCount === right.failedAssertionCount
    && left.commonPrefixBytes === right.commonPrefixBytes
    && left.estimatedCommonPrefixTokens === right.estimatedCommonPrefixTokens
    && left.nextRequestBytes === right.nextRequestBytes
    && left.uncachedSuffixBytes === right.uncachedSuffixBytes
    && left.cacheEpochInvalidationTransitionCount === right.cacheEpochInvalidationTransitionCount
    && left.weightedNextRequestReuseRatio === right.weightedNextRequestReuseRatio
}

async function readSourceRevision(): Promise<string> {
  const sourceRevision = await new Promise<string>((resolve, reject) => {
    execFile(
      "git",
      ["rev-parse", "--verify", "HEAD"],
      { cwd: packageRoot, encoding: "utf8" },
      (error, stdout) => {
        if (error) {
          reject(new Error("prompt_cache_benchmark_source_revision_unavailable", { cause: error }))
          return
        }
        resolve(stdout)
      },
    )
  })
  return normalizeSourceRevision(sourceRevision)
}

function normalizeSourceRevision(value: string): string {
  const normalized = value.trim().toLowerCase()
  if (!isSourceRevision(normalized)) throw new Error("prompt_cache_benchmark_source_revision_invalid")
  return normalized
}

async function digestSourceFiles(
  relativePaths: readonly string[],
  allowMissing = false,
): Promise<string> {
  const digest = createHash("sha256")
  for (const relativePath of [...relativePaths].sort()) {
    digest.update(relativePath, "utf8")
    digest.update("\0", "utf8")
    try {
      digest.update(await readFile(new URL(relativePath, packageRoot)))
    }
    catch (error) {
      if (!allowMissing || !isRecord(error) || error.code !== "ENOENT") throw error
      digest.update("<missing>", "utf8")
    }
    digest.update("\0", "utf8")
  }
  return digest.digest("hex")
}

async function sameRunMultiStepScenario(): Promise<PromptCacheBenchmarkScenario> {
  const provider = new CapturingProvider()
  const runtime = runtimeWithCompiler(provider, emptyRepository())
  const current = baseInput({
    runId: "airun_same_run",
    conversationId: "aicnv_same_run",
    input: "检查应用 alpha 的当前状态，并说明下一步。",
    loadedOperationIds: [inspectApplicationTool.operationId],
  })
  await runtime.complete(current)
  await runtime.complete({
    ...current,
    continuationMessages: [
      {
        role: "assistant",
        content: "",
        toolCalls: [{
          id: "call_inspect_alpha",
          operationId: inspectApplicationTool.operationId,
          arguments: { applicationId: "app_alpha" },
        }],
      },
      {
        role: "tool",
        toolCallId: "call_inspect_alpha",
        content: JSON.stringify({ status: "healthy", readyReplicas: 3, desiredReplicas: 3 }),
      },
    ],
  })
  const initial = measuredStep("initial", "首个模型步骤", provider.assistantRequest(0))
  const continued = measuredStep("after-tool", "同 Run 工具结果后的模型步骤", provider.assistantRequest(1))
  return scenario(
    "same-run-multi-step",
    "同 Run 多步",
    "比较同一 Run 在工具结果加入前后的主回答请求，验证稳定上下文与工具契约未改变。",
    [initial, continued],
    [
      assertion("stable-message-prefix", "新增 continuation 前的消息保持逐项一致", messagesStartWith(continued.request.messages, initial.request.messages)),
      assertion("stable-tool-contract", "两个步骤的工具定义完全一致", sameJSON(initial.request.tools, continued.request.tools)),
      assertion("tool-result-preserved", "后续步骤保留工具调用与工具结果", requestContains(continued.request, "call_inspect_alpha") && requestContains(continued.request, "readyReplicas")),
      assertion("same-conversation", "两个步骤绑定同一会话", initial.request.conversationId === continued.request.conversationId),
    ],
  )
}

async function crossTurnHistoryScenario(): Promise<PromptCacheBenchmarkScenario> {
  const provider = new CapturingProvider()
  const runtime = runtimeWithCompiler(provider, emptyRepository())
  const firstQuestion = "确认应用 alpha 的期望副本数。"
  const firstAnswer = "应用 alpha 的期望副本数为 3。"
  const firstPageContext = { projectId: "prj_benchmark", route: "/applications/app_alpha" }
  await runtime.complete(baseInput({
    runId: "airun_turn_0",
    conversationId: "aicnv_cross_turn",
    input: firstQuestion,
    pageContext: firstPageContext,
    conversation: { title: "跨轮历史基准", titleSource: "user", turnIndex: 0 },
  }))
  await runtime.complete(baseInput({
    runId: "airun_turn_1",
    conversationId: "aicnv_cross_turn",
    input: "继续核对最新部署状态。",
    history: [{
      turnIndex: 0,
      user: firstQuestion,
      assistant: firstAnswer,
      pageContext: firstPageContext,
    }],
    conversation: { title: "跨轮历史基准", titleSource: "user", turnIndex: 1 },
  }))
  const first = measuredStep("turn-0", "第 0 轮", provider.assistantRequest(0))
  const second = measuredStep("turn-1", "带第 0 轮历史的第 1 轮", provider.assistantRequest(1))
  return scenario(
    "cross-turn-history",
    "跨 Turn 历史",
    "比较相邻 Turn 请求，验证前一轮规范化输入（含页面上下文）转入历史后仍保留用户事实和助手结论。",
    [first, second],
    [
      assertion("history-user-preserved", "后一轮包含上一轮用户消息", requestContains(second.request, firstQuestion)),
      assertion("history-answer-preserved", "后一轮包含上一轮助手结论", requestContains(second.request, firstAnswer)),
      assertion("history-page-context-preserved", "后一轮包含上一轮页面上下文", requestContains(second.request, firstPageContext.route)),
      assertion("new-user-preserved", "后一轮包含当前用户任务", requestContains(second.request, "继续核对最新部署状态")),
      assertion("transient-reference-not-replayed", "历史 Turn 不重放工作流参考，仅当前轮保留一条 transient reference 消息", workflowReferenceMessageCount(second.request) === 1),
      assertion("same-conversation", "相邻 Turn 绑定同一会话", first.request.conversationId === second.request.conversationId),
    ],
  )
}

async function toolTouchAndAdditionScenario(): Promise<PromptCacheBenchmarkScenario> {
  const provider = new CapturingProvider()
  const runtime = runtimeWithCompiler(provider, emptyRepository(), orderedToolResolver)
  const shared = {
    runId: "airun_tool_selection",
    conversationId: "aicnv_tool_selection",
    input: "核对应用 alpha 的状态。",
  }
  await runtime.complete(baseInput({
    ...shared,
    loadedOperationIds: [inspectApplicationTool.operationId, readDeploymentLogsTool.operationId],
  }))
  await runtime.complete(baseInput({
    ...shared,
    loadedOperationIds: [readDeploymentLogsTool.operationId, inspectApplicationTool.operationId],
  }))
  await runtime.complete(baseInput({
    ...shared,
    loadedOperationIds: [readDeploymentLogsTool.operationId, inspectApplicationTool.operationId, restartDeploymentTool.operationId],
  }))
  const baseline = measuredStep("baseline", "初始工具顺序", provider.assistantRequest(0))
  const touched = measuredStep("touched", "触碰既有工具后的 LRU 顺序", provider.assistantRequest(1))
  const added = measuredStep("added", "新增工具后", provider.assistantRequest(2))
  return scenario(
    "tool-touch-and-addition",
    "工具触碰与新增",
    "分别测量既有工具 LRU 顺序变化和新工具加入，同时断言工具 Schema 没有漂移。",
    [baseline, touched, added],
    [
      assertion("touch-keeps-tool-set", "触碰只改变既有工具顺序，不改变集合", sameToolSet(baseline.request.tools, touched.request.tools)),
      assertion("touch-keeps-schemas", "触碰前后每个既有工具定义完全一致", sameToolDefinitions(baseline.request.tools, touched.request.tools)),
      assertion("addition-keeps-existing-schemas", "新增工具不改写既有工具定义", toolDefinitionsContain(added.request.tools, touched.request.tools)),
      assertion("addition-appends-requested-tool", "新增步骤包含 restartDeployment", added.request.tools?.some(tool => tool.operationId === restartDeploymentTool.operationId) === true),
    ],
  )
}

async function compactionScenario(): Promise<PromptCacheBenchmarkScenario> {
  const history = compactionHistory()
  const repository = new MutableContextRepository(history)
  const provider = new CapturingProvider(JSON.stringify(summaryContent))
  const runtime = runtimeWithCompiler(provider, repository)
  const firstInput = "先确认应用 alpha 当前副本与健康状态。"
  const firstAnswer = "已确认应用 alpha 当前健康，期望副本数为 3。"
  const firstPageContext = { projectId: "prj_benchmark", route: "/applications/app_alpha" }
  const firstTurn = baseInput({
    runId: "airun_compaction_turn_5",
    conversationId: "aicnv_compaction",
    input: firstInput,
    pageContext: firstPageContext,
    history,
    conversation: { title: "压缩基准", titleSource: "user", turnIndex: history.length },
    loadedOperationIds: [inspectApplicationTool.operationId],
    model,
  })
  repository.latestUsage = {
    modelId: model.id,
    promptTokens: 1_000,
    maxContextTokensSnapshot: model.maxContextTokens,
  }
  await runtime.complete(firstTurn)

  const firstTurnEntry: ConversationHistoryEntry = {
    turnIndex: history.length,
    user: firstInput,
    assistant: firstAnswer,
    pageContext: firstPageContext,
  }
  const secondHistory = [...history, firstTurnEntry]
  const secondInput = "继续检查部署 dep_alpha 是否 ready，并保持只读。"
  const secondAnswer = "已确认部署 dep_alpha 状态为 ready，未执行写操作。"
  const secondPageContext = {
    projectId: "prj_benchmark",
    route: "/deployments/dep_alpha",
    deploymentId: "dep_alpha",
  }
  repository.latestUsage = {
    modelId: model.id,
    promptTokens: 3_600,
    maxContextTokensSnapshot: model.maxContextTokens,
  }
  await runtime.complete(baseInput({
    runId: "airun_compaction_turn_6",
    conversationId: "aicnv_compaction",
    input: secondInput,
    pageContext: secondPageContext,
    history: secondHistory,
    conversation: { title: "压缩基准", titleSource: "user", turnIndex: secondHistory.length },
    loadedOperationIds: [inspectApplicationTool.operationId],
    model,
  }))

  const secondTurnEntry: ConversationHistoryEntry = {
    turnIndex: secondHistory.length,
    user: secondInput,
    assistant: secondAnswer,
    pageContext: secondPageContext,
  }
  const thirdHistory = [...secondHistory, secondTurnEntry]
  const thirdInput = "汇总已确认事实并列出仍需权威回读的项目。"
  repository.latestUsage = {
    modelId: model.id,
    promptTokens: 1_000,
    maxContextTokensSnapshot: model.maxContextTokens,
  }
  await runtime.complete(baseInput({
    runId: "airun_compaction_turn_7",
    conversationId: "aicnv_compaction",
    input: thirdInput,
    pageContext: { projectId: "prj_benchmark", route: "/deployments" },
    history: thirdHistory,
    conversation: { title: "压缩基准", titleSource: "user", turnIndex: thirdHistory.length },
    loadedOperationIds: [inspectApplicationTool.operationId],
    model,
  }))

  const before = measuredStep("before-compaction", "首次未压缩 Turn", provider.assistantRequest(0))
  const after = measuredStep("after-compaction", "下一自然 Turn 触发压缩", provider.assistantRequest(1))
  const reused = measuredStep("summary-reused", "再下一自然 Turn 复用摘要", provider.assistantRequest(2))
  const afterSummary = messageStartingWith(after.request, "历史会话结构化摘要")
  const reusedSummary = messageStartingWith(reused.request, "历史会话结构化摘要")
  return scenario(
    "before-and-after-compaction",
    "压缩边界与摘要复用",
    "用三个输入与 TurnIndex 均不同的自然相邻 Turn，测量首次跨越缓存 epoch 的压缩失效，以及持久摘要在新 epoch 内的后续复用。",
    [before, after, reused],
    [
      assertion("natural-turn-sequence", "三个步骤使用不同当前任务", [firstInput, secondInput, thirdInput].every((input, index) => requestContains([before, after, reused][index]!.request, input))),
      assertion("before-not-compacted", "首个自然 Turn 尚未压缩", before.request.conversationCompacted === false),
      assertion("compaction-flag", "压缩后的请求带有内部压缩状态", after.request.conversationCompacted === true),
      assertion("summary-reused-flag", "再下一自然 Turn 复用已持久摘要", reused.request.conversationCompacted === true),
      assertion("summary-stable", "压缩摘要在新 cache epoch 内逐字节稳定复用", Boolean(afterSummary) && afterSummary === reusedSummary),
      assertion("references-do-not-accumulate", "自然 Turn 增长和摘要复用均不累计历史工作流参考", [before, after, reused].every(step => workflowReferenceMessageCount(step.request) === 1)),
      assertion("durable-facts-preserved", "压缩与复用请求均保留目标、约束、资源与持久事实", [
        "保持应用 alpha 可用",
        "仅使用只读操作",
        "app_alpha",
        "期望副本数为 3",
      ].every(fact => requestContains(after.request, fact) && requestContains(reused.request, fact))),
      assertion("boundary-retains-recent-turn", "跨越压缩边界后仍保留紧邻的自然 Turn", requestContains(after.request, firstInput) && requestContains(after.request, firstAnswer)),
      assertion("reused-epoch-retains-new-turn", "摘要复用请求继续保留压缩后的新增 Turn", requestContains(reused.request, secondInput) && requestContains(reused.request, secondAnswer)),
      assertion("compressed-history-removed", "压缩后不再携带最早一轮原文", requestContains(before.request, history[0]?.assistant ?? "__missing__") && !requestContains(after.request, history[0]?.assistant ?? "__missing__") && !requestContains(reused.request, history[0]?.assistant ?? "__missing__")),
    ],
    ["cache_epoch_invalidation", "within_epoch"],
  )
}

function scenario(
  id: string,
  title: string,
  description: string,
  steps: MeasuredStep[],
  assertions: PromptCacheBenchmarkAssertion[],
  cacheEpochTransitions: PromptCacheBenchmarkCacheEpochTransition[] = [],
): PromptCacheBenchmarkScenario {
  return {
    id,
    title,
    description,
    steps: steps.map(step => step.result),
    transitions: steps.slice(1).map((step, index) => transition(
      steps[index]!,
      step,
      cacheEpochTransitions[index] ?? "within_epoch",
    )),
    assertions,
  }
}

function measuredStep(id: string, label: string, request: ModelRequest): MeasuredStep {
  const serialized = requestSerializer.serialize(request)
  return {
    request,
    serialized,
    result: {
      id,
      label,
      requestBytes: Buffer.byteLength(serialized, "utf8"),
      requestSha256: createHash("sha256").update(serialized).digest("hex"),
      messageCount: request.messages.length,
      toolCount: request.tools?.length ?? 0,
      conversationCompacted: request.conversationCompacted ?? null,
    },
  }
}

function transition(
  previous: MeasuredStep,
  next: MeasuredStep,
  cacheEpochTransition: PromptCacheBenchmarkCacheEpochTransition,
): PromptCacheBenchmarkTransition {
  const commonPrefixBytes = utf8LongestCommonPrefixBytes(previous.serialized, next.serialized)
  return {
    fromStepId: previous.result.id,
    toStepId: next.result.id,
    commonPrefixBytes,
    estimatedCommonPrefixTokens: estimateTokens(commonPrefixBytes),
    nextRequestReuseRatio: ratio(commonPrefixBytes, next.result.requestBytes),
    uncachedSuffixBytes: next.result.requestBytes - commonPrefixBytes,
    cacheEpochTransition,
  }
}

export function utf8LongestCommonPrefixBytes(left: string, right: string): number {
  const leftBytes = Buffer.from(left, "utf8")
  const rightBytes = Buffer.from(right, "utf8")
  const limit = Math.min(leftBytes.length, rightBytes.length)
  let index = 0
  while (index < limit && leftBytes[index] === rightBytes[index]) index += 1
  return index
}

function estimateTokens(bytes: number): number {
  return Math.ceil(bytes / 4)
}

function ratio(numerator: number, denominator: number): number {
  return denominator > 0 ? Number((numerator / denominator).toFixed(6)) : 0
}

function assertion(id: string, description: string, passed: boolean): PromptCacheBenchmarkAssertion {
  return { id, description, passed }
}

function baseInput(overrides: Partial<AssistantModelInput> = {}): AssistantModelInput {
  return {
    runId: "airun_prompt_cache_benchmark",
    ownerUserId: "usr_prompt_cache_benchmark",
    conversationId: "aicnv_prompt_cache_benchmark",
    input: "核对应用状态。",
    pageContext: { projectId: "prj_benchmark", route: "/applications/app_alpha" },
    history: [],
    conversation: { title: "Prompt Cache Benchmark", titleSource: "user", turnIndex: 0 },
    promptVersion: "system-v4",
    reasoningSummary: "",
    answer: "",
    toolCalls: [],
    continuationMessages: [],
    loadedOperationIds: [],
    toolCatalogDigest: "sha256:prompt-cache-benchmark",
    ...overrides,
  }
}

function runtimeWithCompiler(
  provider: CapturingProvider,
  repository: ContextRepository,
  tools: ModelToolDefinition[] | typeof orderedToolResolver = [inspectApplicationTool],
): ModelRuntime {
  return new ModelRuntime(provider, tools, new ContextCompiler(repository, provider, compilerOptions))
}

type ContextRepository = Pick<Repository,
  "getConversationSummary" | "saveConversationSummary" | "listConversationHistory" | "getLatestReportedModelUsage">

function emptyRepository(): ContextRepository {
  return new MutableContextRepository([])
}

class MutableContextRepository implements ContextRepository {
  latestUsage?: { modelId: string, promptTokens: number, maxContextTokensSnapshot: number }
  private summary?: ConversationSummary

  constructor(private readonly history: ConversationHistoryEntry[]) {}

  async getConversationSummary(): Promise<ConversationSummary | undefined> {
    return this.summary
  }

  async getLatestReportedModelUsage(): Promise<typeof this.latestUsage> {
    return this.latestUsage
  }

  async listConversationHistory(
    _conversationId: string,
    afterTurnIndex: number,
    beforeTurnIndex: number,
    limit: number,
  ): Promise<ConversationHistoryEntry[]> {
    return this.history
      .filter(entry => entry.turnIndex > afterTurnIndex && entry.turnIndex < beforeTurnIndex)
      .slice(0, limit)
  }

  async saveConversationSummary(
    value: Omit<ConversationSummary, "createdAt" | "updatedAt">,
  ): Promise<ConversationSummary> {
    this.summary = {
      ...value,
      createdAt: "2026-08-25T00:00:00.000Z",
      updatedAt: "2026-08-25T00:00:00.000Z",
    }
    return this.summary
  }
}

class CapturingProvider implements ModelProvider {
  readonly assistantRequests: ModelRequest[] = []

  constructor(private readonly summary = JSON.stringify(summaryContent)) {}

  async complete(request: ModelRequest): Promise<ModelResponse> {
    if (request.budget?.operation === "summary") {
      return { text: this.summary, usage: reportedUsage(320, 40) }
    }
    this.assistantRequests.push(request)
    return { text: "benchmark-response", usage: reportedUsage(128, 16) }
  }

  async *stream(request: ModelRequest): AsyncIterable<ModelEvent> {
    this.assistantRequests.push(request)
    yield { type: "completed", usage: reportedUsage(128, 16) }
  }

  capabilities(): ModelCapabilities {
    return { streaming: true, toolCalling: true, structuredOutput: true }
  }

  async health(): Promise<{ ok: boolean }> {
    return { ok: true }
  }

  assistantRequest(index: number): ModelRequest {
    const request = this.assistantRequests[index]
    if (!request) throw new Error(`prompt_cache_benchmark_missing_request:${index}`)
    return request
  }
}

class BenchmarkRequestSerializer extends OpenAIChatCompletionsProvider {
  constructor(options: OpenAIChatCompletionsOptions) {
    super(options)
  }

  serialize(request: ModelRequest): string {
    return JSON.stringify(this.buildRequestBody(request, true))
  }
}

const requestSerializerOptions = {
  baseUrl: "https://benchmark.invalid/v1",
  apiKey: "benchmark-placeholder",
  channelAffinityEnabled: false,
  // 旧 checkout 会忽略这个字段；支持显式 cache key 的实现则纳入真实请求体。
  promptCacheKeyEnabled: true,
  model: model.name,
  timeoutMs: 1_000,
}
const requestSerializer = new BenchmarkRequestSerializer(requestSerializerOptions)

function orderedToolResolver(
  _pageContext: Record<string, unknown>,
  _userInput: string,
  loadedOperationIds: string[],
): ModelToolDefinition[] {
  const tools = new Map([
    [inspectApplicationTool.operationId, inspectApplicationTool],
    [readDeploymentLogsTool.operationId, readDeploymentLogsTool],
    [restartDeploymentTool.operationId, restartDeploymentTool],
  ])
  return loadedOperationIds.flatMap((operationId) => {
    const tool = tools.get(operationId)
    return tool ? [tool] : []
  })
}

function compactionHistory(): ConversationHistoryEntry[] {
  return [
    { turnIndex: 0, user: "请记住目标。", assistant: "目标：保持应用 alpha 可用。" },
    { turnIndex: 1, user: "只允许只读操作。", assistant: "约束：未经确认不得重启。" },
    { turnIndex: 2, user: "应用 ID 是什么？", assistant: "已确认应用 ID 为 app_alpha。" },
    { turnIndex: 3, user: "记录副本数。", assistant: "应用 alpha 的期望副本数为 3。" },
    { turnIndex: 4, user: "项目空间是什么？", assistant: "应用位于项目空间 benchmark。" },
  ]
}

function reportedUsage(inputTokens: number, outputTokens: number) {
  return {
    status: "reported" as const,
    value: { inputTokens, outputTokens, totalTokens: inputTokens + outputTokens },
  }
}

function messagesStartWith(messages: ModelRequest["messages"], prefix: ModelRequest["messages"]): boolean {
  return sameJSON(messages.slice(0, prefix.length), prefix)
}

function requestContains(request: ModelRequest, value: string): boolean {
  return request.messages.some((message) => {
    if (message.content.includes(value)) return true
    if (message.role !== "assistant") return false
    return message.toolCalls?.some(call => call.id?.includes(value)
      || call.operationId.includes(value)
      || JSON.stringify(call.arguments).includes(value)) === true
  })
}

function messageStartingWith(request: ModelRequest, value: string): string | undefined {
  return request.messages.find(message => message.content.startsWith(value))?.content
}

function workflowReferenceMessageCount(request: ModelRequest): number {
  return request.messages.filter(message => message.content.includes("<LUNA_DEVOPS_REFERENCE")).length
}

function sameToolSet(left: ModelToolDefinition[] | undefined, right: ModelToolDefinition[] | undefined): boolean {
  return sameJSON(
    [...(left ?? [])].map(tool => tool.operationId).sort(),
    [...(right ?? [])].map(tool => tool.operationId).sort(),
  )
}

function sameToolDefinitions(left: ModelToolDefinition[] | undefined, right: ModelToolDefinition[] | undefined): boolean {
  return toolDefinitionsContain(left, right) && toolDefinitionsContain(right, left)
}

function toolDefinitionsContain(
  candidate: ModelToolDefinition[] | undefined,
  expected: ModelToolDefinition[] | undefined,
): boolean {
  const definitions = new Map((candidate ?? []).map(tool => [tool.operationId, tool]))
  return (expected ?? []).every(tool => sameJSON(definitions.get(tool.operationId), tool))
}

function sameJSON(left: unknown, right: unknown): boolean {
  return JSON.stringify(left) === JSON.stringify(right)
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value)
}

function hasExactKeys(value: Record<string, unknown>, expected: readonly string[]): boolean {
  const keys = Object.keys(value)
  return keys.length === expected.length && expected.every(key => Object.hasOwn(value, key))
}

function hasUniqueStrings<T>(values: T[], select: (value: T) => unknown): boolean {
  const selected = values.map(select)
  return selected.every(isNonEmptyString) && new Set(selected).size === selected.length
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === "string" && value.trim().length > 0
}

function isPositiveInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isInteger(value) && value > 0
}

function isNonNegativeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isInteger(value) && value >= 0
}

function isSha256(value: unknown): value is string {
  return typeof value === "string" && /^[a-f0-9]{64}$/.test(value)
}

function isSourceRevision(value: unknown): value is string {
  return typeof value === "string" && /^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(value)
}

function isCacheEpochTransition(value: unknown): value is PromptCacheBenchmarkCacheEpochTransition {
  return value === "within_epoch" || value === "cache_epoch_invalidation"
}
