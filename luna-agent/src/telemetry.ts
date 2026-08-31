import { AsyncLocalStorage } from "node:async_hooks"
import { existsSync } from "node:fs"
import { createRequire } from "node:module"
import type { Writable } from "node:stream"
import { context, metrics, propagation, ROOT_CONTEXT, SpanKind, SpanStatusCode, trace, type Attributes, type Context, type Counter, type Histogram, type Span, type SpanOptions, type UpDownCounter } from "@opentelemetry/api"
import { FastifyOtelInstrumentation } from "@fastify/otel"
import { OTLPLogExporter } from "@opentelemetry/exporter-logs-otlp-http"
import { OTLPMetricExporter } from "@opentelemetry/exporter-metrics-otlp-http"
import { OTLPTraceExporter } from "@opentelemetry/exporter-trace-otlp-http"
import { HttpInstrumentation } from "@opentelemetry/instrumentation-http"
import { PgInstrumentation } from "@opentelemetry/instrumentation-pg"
import { PinoInstrumentation } from "@opentelemetry/instrumentation-pino"
import { UndiciInstrumentation } from "@opentelemetry/instrumentation-undici"
import { resourceFromAttributes } from "@opentelemetry/resources"
import { BatchLogRecordProcessor } from "@opentelemetry/sdk-logs"
import { PeriodicExportingMetricReader } from "@opentelemetry/sdk-metrics"
import { NodeSDK } from "@opentelemetry/sdk-node"
import type pinoFactory from "pino"
import type { DestinationStream, Logger } from "pino"
import type pinoPrettyFactory from "pino-pretty"
import type { Pool } from "pg"
import { loadTelemetryConfig, type AgentTelemetryConfig } from "./config.js"
import { findDiagnosticError } from "./diagnostic-error.js"
import { configureGenAIAgentVersion, genAISchemaURL } from "./genai-semconv.js"
import { redact } from "./redaction.js"

const instrumentationName = "luna-agent"
const tracer = trace.getTracerProvider().getTracer(instrumentationName, undefined, { schemaUrl: genAISchemaURL })
const require = createRequire(import.meta.url)
const aiCorrelationStorage = new AsyncLocalStorage<Attributes>()
const aiCorrelationAttributeNames = [
  "gen_ai.conversation.id",
  "luna.turn.id",
  "luna.run.id",
] as const

let sdk: NodeSDK | undefined
let processLogger: Logger | undefined
let aiContentCaptureEnabled = false
let agentConfig: AgentTelemetryConfig | undefined
const aiContentAttributeLimitBytes = 128 * 1024

const safeDefaultConfig = loadTelemetryConfig({})

export function configureAgentTelemetry(config: AgentTelemetryConfig): void {
  agentConfig = config
  processLogger = undefined
  configureGenAIAgentVersion(config.OTEL_SERVICE_VERSION)
}

export function configureAIContentCapture(enabled: boolean): void {
  aiContentCaptureEnabled = enabled
}

export function isAIContentCaptureEnabled(): boolean {
  return aiContentCaptureEnabled
}

export function serializeAIContent(value: unknown, limitBytes = aiContentAttributeLimitBytes): { value: string, truncated: boolean, byteLength: number } {
  let serialized: string
  try {
    serialized = JSON.stringify(value, (_key, item: unknown) => typeof item === "bigint" ? item.toString() : item) ?? "null"
  } catch {
    serialized = "[UNSERIALIZABLE]"
  }
  const byteLength = Buffer.byteLength(serialized, "utf8")
  return { value: serialized, truncated: byteLength > limitBytes, byteLength }
}

export function recordAIContent(
  span: Span,
  eventName: string,
  attributeName: string,
  value: unknown,
  attributes: Attributes = {},
): void {
  if (!aiContentCaptureEnabled) return
  const content = serializeAIContent(value)
  if (content.truncated) {
    span.setAttribute("luna.ai.content.truncated", true)
    span.addEvent("luna.ai.content.omitted", {
      "luna.ai.content.field": attributeName,
      "luna.ai.content.size_bytes": content.byteLength,
      "luna.ai.content.limit_bytes": aiContentAttributeLimitBytes,
    })
    telemetryLog(eventName, "debug", {
      ...attributes,
      "luna.ai.content.field": attributeName,
      "luna.ai.content.size_bytes": content.byteLength,
      "luna.ai.content.limit_bytes": aiContentAttributeLimitBytes,
      "luna.ai.content.truncated": true,
    })
    return
  }
  const contentAttributes: Attributes = {
    ...activeAICorrelationAttributes(),
    ...attributes,
  }
  span.setAttribute(attributeName, content.value)
  telemetryLog(eventName, "debug", contentAttributes)
}

export const genAIClientTokenUsageMetric = {
  name: "gen_ai.client.token.usage",
  description: "Number of input and output tokens used.",
  unit: "{token}",
  explicitBucketBoundaries: [1, 4, 16, 64, 256, 1_024, 4_096, 16_384, 65_536, 262_144, 1_048_576, 4_194_304, 16_777_216, 67_108_864],
}

export const agentMetrics = {
  runs: deferredCounter("luna_devops_agent_runs", "Agent 运行次数"),
  runDuration: deferredHistogram("luna_devops_agent_run_duration", "Agent 运行耗时", "s"),
  activeRuns: deferredUpDownCounter("luna_devops_agent_active_runs", "当前正在执行的 Agent 运行数"),
  modelRequests: deferredCounter("luna_devops_agent_model_requests", "模型请求次数"),
  modelDuration: deferredHistogram("luna_devops_agent_model_request_duration", "模型请求耗时", "s"),
  modelFirstTokenDuration: deferredHistogram("luna_devops_agent_model_first_token_duration", "模型首个输出片段耗时", "s"),
  modelSteps: deferredCounter("luna_devops_agent_model_steps", "Agent 模型循环轮次"),
  modelTokenUsage: deferredHistogram(
    genAIClientTokenUsageMetric.name,
    genAIClientTokenUsageMetric.description,
    genAIClientTokenUsageMetric.unit,
    genAIClientTokenUsageMetric.explicitBucketBoundaries,
  ),
  toolCalls: deferredCounter("luna_devops_agent_tool_calls", "工具调用次数"),
  toolDuration: deferredHistogram("luna_devops_agent_tool_call_duration", "工具调用耗时", "s"),
  toolSearches: deferredCounter("luna_devops_agent_tool_searches", "工具目录检索次数"),
  toolSearchMatches: deferredHistogram("luna_devops_agent_tool_search_matches", "单次工具目录检索命中数量", "tool"),
  toolDetailLoads: deferredCounter("luna_devops_agent_tool_detail_loads", "工具详情加载次数"),
  toolDetailItems: deferredHistogram("luna_devops_agent_tool_detail_items", "单次工具详情加载数量", "tool"),
  toolRetrievals: deferredCounter("luna_devops_agent_tool_retrieval_total", "自动工具检索次数"),
  toolRetrievalCandidates: deferredHistogram("luna_devops_agent_tool_retrieval_candidates", "单次自动工具检索候选数量", "tool"),
  toolRetrievalLoaded: deferredHistogram("luna_devops_agent_tool_retrieval_loaded", "单次自动工具检索最终加载数量", "tool"),
  toolRetrievalDuration: deferredHistogram("luna_devops_agent_tool_retrieval_duration", "自动工具检索耗时", "s"),
  toolCatalogRefreshes: deferredCounter("luna_devops_agent_tool_catalog_refreshes", "工具目录热刷新次数"),
  approvals: deferredCounter("luna_devops_agent_approval_decisions", "工具审批决策次数"),
  cards: deferredCounter("luna_devops_agent_interaction_cards", "交互卡片生成次数"),
  externalRequests: deferredCounter("luna_devops_agent_external_requests", "外部请求次数"),
  contextCompilations: deferredCounter("luna_devops_agent_context_compilations", "上下文编译次数"),
  contextCompressionDuration: deferredHistogram("luna_devops_agent_context_compression_duration", "上下文压缩耗时", "s"),
  streamEvents: deferredCounter("luna_devops_agent_stream_events", "Agent 实时事件发布次数"),
  streamTransportDuration: deferredHistogram("luna_devops_agent_stream_transport_duration", "Agent 进程内实时流操作耗时", "s"),
  streamBufferBytes: deferredHistogram("luna_devops_agent_stream_event_bytes", "Agent 进程内实时流单事件字节数", "By"),
  streamRetainedBytes: deferredHistogram("luna_devops_agent_stream_retained_bytes", "Agent 进程内实时流缓冲字节数", "By"),
  activeStreamReaders: deferredUpDownCounter("luna_devops_agent_stream_active_readers", "当前 Agent 进程内实时流读取器数"),
  streamCleanups: deferredCounter("luna_devops_agent_stream_cleanups", "Agent 进程内终态缓冲清理次数"),
  sseBackpressureDuration: deferredHistogram("luna_devops_agent_sse_backpressure_duration", "Agent SSE 背压等待耗时", "s"),
  activeSseSubscribers: deferredUpDownCounter("luna_devops_agent_sse_active_subscribers", "当前 Agent SSE 订阅者数"),
  sseSubscriptions: deferredCounter("luna_devops_agent_sse_subscriptions", "Agent SSE 订阅结果"),
  databasePoolAcquireDuration: deferredHistogram("luna_devops_agent_database_pool_acquire_duration", "Agent PostgreSQL 获取连接等待耗时", "s"),
  streamPersistenceDuration: deferredHistogram("luna_devops_agent_stream_persistence_duration", "Agent 流式终态批量持久化耗时", "s"),
  streamTerminalPersistence: deferredCounter("luna_devops_agent_stream_terminal_persistence", "Agent 终态持久化尝试次数"),
}

const observedDatabasePools = new WeakSet<Pool>()

/** PostgreSQL 池指标只包含稳定状态枚举，不暴露用户、Run 或实例标识。 */
export function observeDatabasePool(pool: Pool): void {
  if (observedDatabasePools.has(pool)) return
  observedDatabasePools.add(pool)
  const meter = metrics.getMeter(instrumentationName)
  meter.createObservableGauge("luna_devops_agent_database_pool_connections", {
    description: "Agent PostgreSQL 连接池连接数",
    unit: "connection",
  }).addCallback((result) => {
    result.observe(pool.totalCount, { state: "total" })
    result.observe(pool.idleCount, { state: "idle" })
    result.observe(Math.max(0, pool.totalCount - pool.idleCount), { state: "active" })
  })
  meter.createObservableGauge("luna_devops_agent_database_pool_waiting", {
    description: "等待 Agent PostgreSQL 连接的请求数",
    unit: "request",
  }).addCallback(result => result.observe(pool.waitingCount))
}

function deferredCounter(name: string, description: string): Pick<Counter, "add"> {
  let instrument: Counter | undefined
  return { add: (value, attributes, activeContext) => (instrument ??= metrics.getMeter(instrumentationName).createCounter(name, { description })).add(value, attributes, activeContext) }
}

function deferredUpDownCounter(name: string, description: string): Pick<UpDownCounter, "add"> {
  let instrument: UpDownCounter | undefined
  return { add: (value, attributes, activeContext) => (instrument ??= metrics.getMeter(instrumentationName).createUpDownCounter(name, { description })).add(value, attributes, activeContext) }
}

function deferredHistogram(name: string, description: string, unit?: string, explicitBucketBoundaries?: number[]): Pick<Histogram, "record"> {
  let instrument: Histogram | undefined
  return {
    record: (value, attributes, activeContext) => (instrument ??= metrics.getMeter(instrumentationName).createHistogram(name, {
      description,
      ...(unit ? { unit } : {}),
      ...(explicitBucketBoundaries ? { advice: { explicitBucketBoundaries } } : {}),
    })).record(value, attributes, activeContext),
  }
}

export function initializeTelemetry(config: AgentTelemetryConfig): void {
  configureAgentTelemetry(config)
  const endpoint = config.OTEL_EXPORTER_OTLP_ENDPOINT
  if (!endpoint?.trim() || sdk) return
  const parsedEndpoint = new URL(endpoint)
  if (parsedEndpoint.protocol !== "http:" && parsedEndpoint.protocol !== "https:") {
    throw new Error("OTEL_EXPORTER_OTLP_ENDPOINT must use http or https")
  }
  const headers = parseOTELKeyValueList(config.OTEL_EXPORTER_OTLP_HEADERS)
  const resourceAttributes = parseOTELKeyValueList(config.OTEL_RESOURCE_ATTRIBUTES)
  if (config.OTEL_SERVICE_VERSION) resourceAttributes["service.version"] = config.OTEL_SERVICE_VERSION.trim()
  sdk = new NodeSDK({
    serviceName: "luna-agent",
    resource: resourceFromAttributes(resourceAttributes),
    resourceDetectors: [],
    traceExporter: new OTLPTraceExporter({ url: otlpSignalEndpoint(parsedEndpoint, "traces"), headers }),
    metricReaders: [new PeriodicExportingMetricReader({
      exporter: new OTLPMetricExporter({ url: otlpSignalEndpoint(parsedEndpoint, "metrics"), headers }),
      exportIntervalMillis: 15_000,
    })],
    logRecordProcessors: [new BatchLogRecordProcessor({ exporter: new OTLPLogExporter({ url: otlpSignalEndpoint(parsedEndpoint, "logs"), headers }) })],
    instrumentations: [
      new HttpInstrumentation({
        ignoreIncomingRequestHook: request => isHealthCheckPath(request.url ?? ""),
        requestHook: (span, request) => sanitizeRequestSpan(span, request),
        applyCustomAttributesOnSpan: (span, request) => sanitizeRequestSpan(span, request),
      }),
      new FastifyOtelInstrumentation({
        registerOnInitialization: true,
        instrumentHooks: false,
        ignorePaths: route => isHealthCheckPath(route.url),
        requestHook: (span, request) => sanitizeSpanURL(span, request.raw.url ?? request.url),
        lifecycleHook: (span, { hookName, request }) => {
          const route = request.routeOptions.url
          const method = Array.isArray(request.routeOptions.method)
            ? request.routeOptions.method.join("|")
            : request.routeOptions.method ?? ""
          const stableName = stableFastifyLifecycleSpanName(hookName ?? "", method, route ?? "")
          span.updateName(stableName)
          span.setAttribute("hook.name", stableName)
          span.setAttribute("hook.callback.name", "fastify.route_handler")
        },
      }),
      ...(isDatabaseSpanCaptureEnabled(config.AI_OBSERVABILITY_CAPTURE_DATABASE_SPANS)
        ? [new PgInstrumentation({
            enhancedDatabaseReporting: false,
            ignoreConnectSpans: true,
            requireParentSpan: true,
            responseHook: (span, response) => {
              span.setAttribute("db.response.returned_rows", response.data.rowCount ?? 0)
            },
          })]
        : []),
      new PinoInstrumentation({
        disableLogCorrelation: false,
        disableLogSending: false,
        logKeys: { traceId: "trace_id", spanId: "span_id", traceFlags: "trace_flags" },
      }),
      new UndiciInstrumentation({
        requestHook: (span, request) => {
          const origin = typeof request.origin === "string" ? request.origin : String(request.origin)
          sanitizeSpanURL(span, `${origin}${request.path}`)
        },
        responseHook: (span, { request }) => {
          const origin = typeof request.origin === "string" ? request.origin : String(request.origin)
          sanitizeSpanURL(span, `${origin}${request.path}`)
        },
      }),
    ],
  })
  sdk.start()
}

export function isDatabaseSpanCaptureEnabled(value = false): boolean {
  return value
}

function otlpSignalEndpoint(base: URL, signal: "logs" | "metrics" | "traces"): string {
  const endpoint = new URL(base)
  endpoint.pathname = `${endpoint.pathname.replace(/\/+$/, "")}/v1/${signal}`
  return endpoint.toString()
}

function parseOTELKeyValueList(value?: string): Record<string, string> {
  if (!value) return {}
  return Object.fromEntries(value.split(",").map((entry) => {
    const separator = entry.indexOf("=")
    return [decodeURIComponent(entry.slice(0, separator).trim()), decodeURIComponent(entry.slice(separator + 1).trim())]
  }))
}

export function isHealthCheckPath(value: string): boolean {
  try {
    const path = new URL(value, "http://health-check.invalid").pathname
    return path === "/healthz" || path === "/internal/health/live" || path === "/internal/health/ready"
  }
  catch {
    return false
  }
}

export function stableFastifyLifecycleSpanName(hookName: string, method: string, route: string): string {
  const stableHook = /^[A-Za-z][A-Za-z0-9_-]{0,40}$/.test(hookName) ? hookName : "handler"
  const stableMethod = /^[A-Z|]{3,48}$/.test(method) ? method : "UNKNOWN"
  const stableRoute = (route.split(/[?#]/, 1)[0] || "/unknown").slice(0, 256)
  return `fastify.${stableHook} ${stableMethod} ${stableRoute}`
}

export function sanitizeTelemetryURL(value: string): string {
  try {
    const parsed = new URL(value)
    parsed.username = ""
    parsed.password = ""
    parsed.search = ""
    parsed.hash = ""
    return parsed.toString()
  }
  catch {
    return value.split(/[?#]/, 1)[0] ?? ""
  }
}

function sanitizeRequestSpan(span: Span, request: unknown): void {
  if (!request || typeof request !== "object") return
  if ("url" in request && typeof request.url === "string") {
    sanitizeSpanURL(span, request.url)
    return
  }
  if ("path" in request && typeof request.path === "string") sanitizeSpanURL(span, request.path)
}

function sanitizeSpanURL(span: Span, value: string): void {
  try {
    const absolute = /^[a-z][a-z0-9+.-]*:\/\//i.test(value)
    const parsed = new URL(value, "http://telemetry.invalid")
    span.setAttribute("url.path", parsed.pathname)
    span.setAttribute("url.query", "")
    if (absolute) span.setAttribute("url.full", sanitizeTelemetryURL(value))
  }
  catch {
    span.setAttribute("url.query", "")
  }
}

export async function shutdownTelemetry(): Promise<void> {
  const current = sdk
  sdk = undefined
  if (current) await current.shutdown()
}

export async function withSpan<T>(
  name: string,
  options: SpanOptions,
  operation: (span: Span) => Promise<T>,
  parentContext: Context = context.active(),
): Promise<T> {
  return tracer.startActiveSpan(name, options, parentContext, async span => {
    const correlationAttributes = mergeAICorrelationAttributes(activeAICorrelationAttributes(), options.attributes)
    return aiCorrelationStorage.run(correlationAttributes, async () => {
      try {
        return await operation(span)
      }
      catch (error) {
        if (isExpectedCancellation(error)) {
          span.setAttribute("luna.operation.outcome", "cancelled")
        } else {
          recordSpanError(span, error)
        }
        throw error
      }
      finally {
        span.end()
      }
    })
  })
}

export async function* withSpanStream<T>(
  name: string,
  options: SpanOptions,
  operation: (span: Span) => AsyncIterable<T>,
  parentContext: Context = context.active(),
): AsyncIterable<T> {
  const span = tracer.startSpan(name, options, parentContext)
  const spanContext = trace.setSpan(parentContext, span)
  const correlationAttributes = mergeAICorrelationAttributes(activeAICorrelationAttributes(), options.attributes)
  const iterator = operation(span)[Symbol.asyncIterator]()
  let completed = false
  try {
    while (true) {
      const next = await context.with(spanContext, () => aiCorrelationStorage.run(correlationAttributes, () => iterator.next()))
      if (next.done) {
        completed = true
        return
      }
      yield next.value
    }
  }
  catch (error) {
    if (isExpectedCancellation(error)) span.setAttribute("luna.operation.outcome", "cancelled")
    else recordSpanError(span, error)
    throw error
  }
  finally {
    if (!completed && iterator.return) {
      await context.with(spanContext, () => aiCorrelationStorage.run(correlationAttributes, () => iterator.return!()))
        .catch(() => undefined)
    }
    span.end()
  }
}

export function isExpectedCancellation(error: unknown): boolean {
  const code = stableErrorCode(error)
  return code === "ai.run_canceled" || code === "ai.agent_stopping"
}

function activeAICorrelationAttributes(): Attributes {
  return aiCorrelationStorage.getStore() ?? {}
}

function mergeAICorrelationAttributes(inherited: Attributes, current: Attributes | undefined): Attributes {
  const merged: Attributes = { ...inherited }
  for (const name of aiCorrelationAttributeNames) {
    const value = current?.[name]
    if (typeof value === "string" && value.length > 0) merged[name] = value
  }
  return merged
}

const traceContextKeys = new Set(["traceparent", "tracestate"])

export function captureTraceContext(fallbackCarrier: Record<string, string | string[] | undefined> = {}): Record<string, string> {
  const carrier: Record<string, string> = {}
  propagation.inject(context.active(), carrier)
  const active = normalizeTraceContext(carrier)
  if (active.traceparent) return active
  const fallback = Object.fromEntries(Object.entries(fallbackCarrier)
    .flatMap(([key, value]) => typeof value === "string" ? [[key, value]] : []))
  return normalizeTraceContext(fallback)
}

export function extractTraceContext(carrier: Record<string, string> | undefined): Context {
  return propagation.extract(ROOT_CONTEXT, normalizeTraceContext(carrier ?? {}))
}

export function normalizeTraceContext(carrier: Record<string, string>): Record<string, string> {
  return Object.fromEntries(Object.entries(carrier)
    .map(([key, value]) => [key.toLowerCase(), value.trim()] as const)
    .filter(([key, value]) => traceContextKeys.has(key) && value.length > 0 && value.length <= 512))
}

export function recordSpanError(span: Span, error: unknown): void {
  const code = stableErrorCode(error)
  const diagnostic = errorDiagnostic(error, code)
  span.setStatus({ code: SpanStatusCode.ERROR, message: code })
  const errorType = diagnostic["error.type"]
  if (errorType !== undefined) span.setAttribute("error.type", errorType)
  span.setAttribute("error.code", code)
  const errorMessage = diagnostic["error.message"]
  if (errorMessage !== undefined) span.setAttribute("error.message", errorMessage)
  const stacktrace = diagnostic["exception.stacktrace"]
  if (stacktrace !== undefined) span.setAttribute("exception.stacktrace", stacktrace)
}

export function stableErrorCode(error: unknown): string {
  const message = error instanceof Error ? error.message : String(error)
  return /^[a-z][a-z0-9_.-]{2,120}$/.test(message) ? message : "ai.internal_error"
}

export function telemetryLog(
  eventName: string,
  severity: "debug" | "info" | "warn" | "error",
  attributes: Attributes = {},
  message = eventName,
): void {
  const activeSpan = trace.getSpan(context.active())
  const spanContext = activeSpan?.spanContext()
  const businessAttributes = { ...activeAICorrelationAttributes(), ...attributes }
  const correlatedAttributes = redact({
    "event.name": eventName,
    ...businessAttributes,
    ...inferResourceAttributes(businessAttributes),
    ...(spanContext ? { trace_id: spanContext.traceId, span_id: spanContext.spanId } : {}),
  })
  agentLogger()[severity](correlatedAttributes, message)
}

function inferResourceAttributes(attributes: Attributes): Attributes {
  if (typeof attributes["resource.id"] === "string") return {}
  const candidates = [
    ["luna.tool_call.id", "agent_tool_call"],
    ["luna.run.id", "agent_run"],
    ["gen_ai.conversation.id", "agent_conversation"],
  ] as const
  for (const [idKey, resourceType] of candidates) {
    const resourceId = attributes[idKey]
    if (typeof resourceId === "string" && resourceId.length > 0)
      return { "resource.type": resourceType, "resource.id": resourceId }
  }
  return {}
}

export function errorDiagnostic(error: unknown, fallbackCode = "ai.internal_error", hint?: string): Attributes {
  const normalized = error instanceof Error ? error : new Error(String(error))
  const embeddedDiagnostic = findDiagnosticError(error)
  const stableCode = stableErrorCode(error)
  const code = embeddedDiagnostic?.code ?? (stableCode === "ai.internal_error" ? fallbackCode : stableCode)
  const resolvedHint = embeddedDiagnostic?.hint ?? hint
  const attributes: Attributes = {
    "error.code": code,
    "error.type": normalized.name || "UnknownError",
    "error.message": redact(errorChain(normalized)),
    ...(resolvedHint ? { "error.hint": resolvedHint } : {}),
  }
  if (normalized.stack) attributes["exception.stacktrace"] = redact(normalized.stack)
  return attributes
}

export function agentLogger(): Logger {
  return processLogger ??= createAgentLogger()
}

export function createAgentLogger(options: {
  color?: string
  destination?: Writable
  format?: string
  isContainer?: boolean
  isTTY?: boolean
  level?: string
  noColor?: boolean
} = {}): Logger {
  const config = currentAgentConfig()
  const pino = require("pino") as typeof pinoFactory
  const destination = options.destination ?? process.stderr
  const format = resolveAgentLogFormat(
    options.format ?? config.LOG_FORMAT,
    options.isTTY ?? Boolean(process.stderr.isTTY),
    options.isContainer ?? agentRunsInContainer(),
  )
  const color = format === "console" && resolveAgentLogColor(
    options.color ?? config.LOG_COLOR,
    options.isTTY ?? Boolean(process.stderr.isTTY),
    options.noColor ?? config.NO_COLOR !== undefined,
  )
  let output: DestinationStream = destination
  if (format === "console") {
    const pretty = require("pino-pretty") as typeof pinoPrettyFactory
    const prettyStream = pretty({
      colorize: color,
      destination,
      hideObject: false,
      ignore: "pid,hostname,service.name,time",
      messageKey: "message",
      messageFormat: (log, messageKey) => singleLineConsoleMessage(log[messageKey]),
      singleLine: true,
    })
    output = prettyStream
  }
  return pino({
    base: { "service.name": "luna-agent" },
    level: resolveAgentLogLevel(options.level ?? config.LOG_LEVEL),
    messageKey: "message",
    redact: {
      censor: "[REDACTED]",
      paths: [
        "authorization", "cookie", "password", "secret", "token", "apiKey", "api_key",
        "accessToken", "access_token", "refreshToken", "refresh_token", "privateKey", "private_key",
        "req.headers.authorization", "req.headers.cookie", "req.body", "request.body",
      ],
    },
    timestamp: pino.stdTimeFunctions.isoTime,
  }, output)
}

function currentAgentConfig(): AgentTelemetryConfig {
  return agentConfig ?? safeDefaultConfig
}

function singleLineConsoleMessage(value: unknown): string {
  const message = typeof value === "string"
    ? value
    : typeof value === "number" || typeof value === "boolean" ? `${value}` : ""
  return message.replaceAll("\r", "\\r").replaceAll("\n", "\\n")
}

export function resolveAgentLogFormat(value: string | undefined, isTTY: boolean, isContainer: boolean): "console" | "json" {
  const normalized = value?.trim().toLowerCase()
  if (normalized === "console" || normalized === "json") return normalized
  return isTTY && !isContainer ? "console" : "json"
}

export function resolveAgentLogColor(value: string | undefined, isTTY: boolean, noColor: boolean): boolean {
  if (noColor) return false
  const normalized = value?.trim().toLowerCase()
  if (normalized === "always") return true
  if (normalized === "never") return false
  return isTTY
}

export function resolveAgentLogLevel(value: string | undefined): "debug" | "info" | "warn" | "error" {
  const normalized = value?.trim().toLowerCase()
  return normalized === "debug" || normalized === "warn" || normalized === "error" ? normalized : "info"
}

function agentRunsInContainer(): boolean {
  return Boolean(process.env.KUBERNETES_SERVICE_HOST || process.env.container || process.platform === "linux" && existsSync("/.dockerenv"))
}

function errorChain(error: Error): string {
  const messages: string[] = []
  const seen = new Set<unknown>()
  let current: unknown = error
  while (current instanceof Error && !seen.has(current)) {
    seen.add(current)
    if (current.message && messages.at(-1) !== current.message) messages.push(current.message)
    current = current.cause
  }
  if (typeof current === "string" || typeof current === "number" || typeof current === "boolean")
    messages.push(String(current))
  return messages.join(": ")
}

export function serverSpanOptions(attributes: Attributes = {}): SpanOptions {
  return { kind: SpanKind.SERVER, attributes }
}

export function internalSpanOptions(attributes: Attributes = {}): SpanOptions {
  return { kind: SpanKind.INTERNAL, attributes }
}

export function clientSpanOptions(attributes: Attributes = {}): SpanOptions {
  return { kind: SpanKind.CLIENT, attributes }
}
