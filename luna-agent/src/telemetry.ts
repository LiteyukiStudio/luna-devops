import { AsyncLocalStorage } from "node:async_hooks"
import { context, metrics, propagation, ROOT_CONTEXT, SpanKind, SpanStatusCode, trace, type Attributes, type Context, type Counter, type Histogram, type Span, type SpanOptions, type UpDownCounter } from "@opentelemetry/api"
import { logs, SeverityNumber } from "@opentelemetry/api-logs"
import { FastifyOtelInstrumentation } from "@fastify/otel"
import { OTLPLogExporter } from "@opentelemetry/exporter-logs-otlp-http"
import { OTLPMetricExporter } from "@opentelemetry/exporter-metrics-otlp-http"
import { OTLPTraceExporter } from "@opentelemetry/exporter-trace-otlp-http"
import { HttpInstrumentation } from "@opentelemetry/instrumentation-http"
import { PgInstrumentation } from "@opentelemetry/instrumentation-pg"
import { PinoInstrumentation } from "@opentelemetry/instrumentation-pino"
import { UndiciInstrumentation } from "@opentelemetry/instrumentation-undici"
import { envDetector } from "@opentelemetry/resources"
import { BatchLogRecordProcessor } from "@opentelemetry/sdk-logs"
import { PeriodicExportingMetricReader } from "@opentelemetry/sdk-metrics"
import { NodeSDK } from "@opentelemetry/sdk-node"
import { genAISchemaURL } from "./genai-semconv.js"
import { redact } from "./redaction.js"

const instrumentationName = "luna-agent"
const tracer = trace.getTracerProvider().getTracer(instrumentationName, undefined, { schemaUrl: genAISchemaURL })
const logger = logs.getLogger(instrumentationName)
const aiCorrelationStorage = new AsyncLocalStorage<Attributes>()
const aiCorrelationAttributeNames = [
  "gen_ai.conversation.id",
  "luna.turn.id",
  "luna.run.id",
] as const

let sdk: NodeSDK | undefined
let aiContentCaptureEnabled = false
const aiContentAttributeLimit = 32_768

export function configureAIContentCapture(enabled: boolean): void {
  aiContentCaptureEnabled = enabled
}

export function isAIContentCaptureEnabled(): boolean {
  return aiContentCaptureEnabled
}

export function serializeAIContent(value: unknown, limit = aiContentAttributeLimit): { value: string, truncated: boolean } {
  let serialized: string
  try {
    serialized = JSON.stringify(redact(value), (_key, item: unknown) => typeof item === "bigint" ? item.toString() : item) ?? "null"
  } catch {
    serialized = "[UNSERIALIZABLE]"
  }
  if (serialized.length <= limit) return { value: serialized, truncated: false }
  return { value: `${serialized.slice(0, Math.max(0, limit - 1))}…`, truncated: true }
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
    telemetryLog(eventName, "debug", {
      ...attributes,
      "luna.ai.content.truncated": true,
    })
    return
  }
  const contentAttributes: Attributes = {
    ...activeAICorrelationAttributes(),
    ...attributes,
    [attributeName]: content.value,
    "luna.ai.content.truncated": content.truncated,
  }
  span.setAttribute(attributeName, content.value)
  span.setAttribute("luna.ai.content.truncated", false)
  telemetryLog(eventName, "debug", contentAttributes)
}

export function recordAvailableTools(tools: Array<{ operationId: string }>): void {
  const operationIds = [...new Set(tools.map(tool => tool.operationId))].sort()
  const span = tracer.startSpan("agent.tools.available", internalSpanOptions({
    "luna.agent.available_tool.count": operationIds.length,
    "luna.agent.available_tool.names": JSON.stringify(operationIds),
  }))
  span.end()
}

export const agentMetrics = {
  runs: deferredCounter("luna_devops_agent_runs", "Agent 运行次数"),
  runDuration: deferredHistogram("luna_devops_agent_run_duration", "Agent 运行耗时", "s"),
  activeRuns: deferredUpDownCounter("luna_devops_agent_active_runs", "当前正在执行的 Agent 运行数"),
  modelRequests: deferredCounter("luna_devops_agent_model_requests", "模型请求次数"),
  modelDuration: deferredHistogram("luna_devops_agent_model_request_duration", "模型请求耗时", "s"),
  modelFirstTokenDuration: deferredHistogram("luna_devops_agent_model_first_token_duration", "模型首个输出片段耗时", "s"),
  modelSteps: deferredCounter("luna_devops_agent_model_steps", "Agent 模型循环轮次"),
  modelTokens: deferredCounter("luna_devops_agent_model_tokens", "模型 Token 用量"),
  toolCalls: deferredCounter("luna_devops_agent_tool_calls", "工具调用次数"),
  toolDuration: deferredHistogram("luna_devops_agent_tool_call_duration", "工具调用耗时", "s"),
  toolSearches: deferredCounter("luna_devops_agent_tool_searches", "工具目录检索次数"),
  toolSearchMatches: deferredHistogram("luna_devops_agent_tool_search_matches", "单次工具目录检索命中数量", "tool"),
  toolDirectoryBrowses: deferredCounter("luna_devops_agent_tool_directory_browse_total", "工具目录确定性浏览次数"),
  toolDirectoryItems: deferredHistogram("luna_devops_agent_tool_directory_items", "单次工具目录浏览返回数量", "tool"),
  toolRetrievals: deferredCounter("luna_devops_agent_tool_retrieval_total", "自动工具检索次数"),
  toolRetrievalCandidates: deferredHistogram("luna_devops_agent_tool_retrieval_candidates", "单次自动工具检索候选数量", "tool"),
  toolRetrievalLoaded: deferredHistogram("luna_devops_agent_tool_retrieval_loaded", "单次自动工具检索最终加载数量", "tool"),
  toolRetrievalDuration: deferredHistogram("luna_devops_agent_tool_retrieval_duration", "自动工具检索耗时", "s"),
  approvals: deferredCounter("luna_devops_agent_approval_decisions", "工具审批决策次数"),
  cards: deferredCounter("luna_devops_agent_interaction_cards", "交互卡片生成次数"),
  externalRequests: deferredCounter("luna_devops_agent_external_requests", "外部请求次数"),
  contextCompilations: deferredCounter("luna_devops_agent_context_compilations", "上下文编译次数"),
  contextInputTokens: deferredHistogram("luna_devops_agent_context_input_tokens", "上下文估算 Token 数", "token"),
  contextCompressionDuration: deferredHistogram("luna_devops_agent_context_compression_duration", "上下文压缩耗时", "s"),
}

function deferredCounter(name: string, description: string): Pick<Counter, "add"> {
  let instrument: Counter | undefined
  return { add: (value, attributes, activeContext) => (instrument ??= metrics.getMeter(instrumentationName).createCounter(name, { description })).add(value, attributes, activeContext) }
}

function deferredUpDownCounter(name: string, description: string): Pick<UpDownCounter, "add"> {
  let instrument: UpDownCounter | undefined
  return { add: (value, attributes, activeContext) => (instrument ??= metrics.getMeter(instrumentationName).createUpDownCounter(name, { description })).add(value, attributes, activeContext) }
}

function deferredHistogram(name: string, description: string, unit?: string): Pick<Histogram, "record"> {
  let instrument: Histogram | undefined
  return { record: (value, attributes, activeContext) => (instrument ??= metrics.getMeter(instrumentationName).createHistogram(name, { description, ...(unit ? { unit } : {}) })).record(value, attributes, activeContext) }
}

export function initializeTelemetry(endpoint = process.env.OTEL_EXPORTER_OTLP_ENDPOINT): void {
  if (!endpoint?.trim() || sdk) return
  const parsedEndpoint = new URL(endpoint)
  if (parsedEndpoint.protocol !== "http:" && parsedEndpoint.protocol !== "https:") {
    throw new Error("OTEL_EXPORTER_OTLP_ENDPOINT must use http or https")
  }
  sdk = new NodeSDK({
    serviceName: "luna-agent",
    resourceDetectors: [envDetector],
    traceExporter: new OTLPTraceExporter(),
    metricReaders: [new PeriodicExportingMetricReader({
      exporter: new OTLPMetricExporter(),
      exportIntervalMillis: 15_000,
    })],
    logRecordProcessors: [new BatchLogRecordProcessor({ exporter: new OTLPLogExporter() })],
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
      }),
      ...(isDatabaseSpanCaptureEnabled()
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

export function isDatabaseSpanCaptureEnabled(value = process.env.AI_OBSERVABILITY_CAPTURE_DATABASE_SPANS): boolean {
  return value?.trim().toLowerCase() === "true"
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
          span.setAttribute("luna.operation.outcome", "canceled")
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
    if (isExpectedCancellation(error)) span.setAttribute("luna.operation.outcome", "canceled")
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
  span.setStatus({ code: SpanStatusCode.ERROR, message: code })
  span.setAttribute("error.type", code === "ai.internal_error"
    ? error instanceof Error ? error.name : "_OTHER"
    : code)
  span.setAttribute("error.code", code)
}

export function stableErrorCode(error: unknown): string {
  const message = error instanceof Error ? error.message : String(error)
  return /^[a-z][a-z0-9_.-]{2,120}$/.test(message) ? message : "ai.internal_error"
}

export function telemetryLog(
  eventName: string,
  severity: "debug" | "info" | "warn" | "error",
  attributes: Attributes = {},
): void {
  const activeSpan = trace.getSpan(context.active())
  const spanContext = activeSpan?.spanContext()
  const correlatedAttributes = {
    "event.name": eventName,
    ...activeAICorrelationAttributes(),
    ...attributes,
    ...(spanContext ? { trace_id: spanContext.traceId, span_id: spanContext.spanId } : {}),
  }
  logger.emit({
    severityNumber: severityNumber(severity),
    severityText: severity.toUpperCase(),
    body: eventName,
    attributes: correlatedAttributes,
  })
  process.stdout.write(`${JSON.stringify({
    level: severity,
    time: new Date().toISOString(),
    message: eventName,
    ...correlatedAttributes,
  })}\n`)
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

function severityNumber(severity: "debug" | "info" | "warn" | "error"): SeverityNumber {
  if (severity === "error") return SeverityNumber.ERROR
  if (severity === "warn") return SeverityNumber.WARN
  if (severity === "debug") return SeverityNumber.DEBUG
  return SeverityNumber.INFO
}
