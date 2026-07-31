import { context, metrics, SpanKind, SpanStatusCode, trace, type Attributes, type Counter, type Histogram, type Span, type SpanOptions, type UpDownCounter } from "@opentelemetry/api"
import { logs, SeverityNumber } from "@opentelemetry/api-logs"
import { getNodeAutoInstrumentations } from "@opentelemetry/auto-instrumentations-node"
import { OTLPLogExporter } from "@opentelemetry/exporter-logs-otlp-http"
import { OTLPMetricExporter } from "@opentelemetry/exporter-metrics-otlp-http"
import { OTLPTraceExporter } from "@opentelemetry/exporter-trace-otlp-http"
import { envDetector } from "@opentelemetry/resources"
import { BatchLogRecordProcessor } from "@opentelemetry/sdk-logs"
import { PeriodicExportingMetricReader } from "@opentelemetry/sdk-metrics"
import { NodeSDK } from "@opentelemetry/sdk-node"

const instrumentationName = "luna-agent"
const tracer = trace.getTracer(instrumentationName)
const logger = logs.getLogger(instrumentationName)

let sdk: NodeSDK | undefined

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
  approvals: deferredCounter("luna_devops_agent_approval_decisions", "工具审批决策次数"),
  cards: deferredCounter("luna_devops_agent_interaction_cards", "交互卡片生成次数"),
  externalRequests: deferredCounter("luna_devops_agent_external_requests", "外部请求次数"),
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
    instrumentations: [getNodeAutoInstrumentations({
      "@opentelemetry/instrumentation-fs": { enabled: false },
      "@opentelemetry/instrumentation-pg": {
        enhancedDatabaseReporting: false,
        responseHook: (span, response) => {
          span.setAttribute("db.response.returned_rows", response.data.rowCount ?? 0)
        },
      },
      "@opentelemetry/instrumentation-pino": {
        disableLogCorrelation: false,
        disableLogSending: false,
        logKeys: { traceId: "trace_id", spanId: "span_id", traceFlags: "trace_flags" },
      },
      "@opentelemetry/instrumentation-undici": {
        requestHook: (span, request) => {
          const origin = typeof request.origin === "string" ? request.origin : String(request.origin)
          span.setAttribute("url.full", sanitizeTelemetryURL(`${origin}${request.path}`))
        },
      },
    })],
  })
  sdk.start()
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

export async function shutdownTelemetry(): Promise<void> {
  const current = sdk
  sdk = undefined
  if (current) await current.shutdown()
}

export async function withSpan<T>(
  name: string,
  options: SpanOptions,
  operation: (span: Span) => Promise<T>,
): Promise<T> {
  return tracer.startActiveSpan(name, options, async span => {
    try {
      return await operation(span)
    }
    catch (error) {
      recordSpanError(span, error)
      throw error
    }
    finally {
      span.end()
    }
  })
}

export function recordSpanError(span: Span, error: unknown): void {
  const code = stableErrorCode(error)
  span.setStatus({ code: SpanStatusCode.ERROR, message: code })
  span.setAttribute("error.type", error instanceof Error ? error.name : "UnknownError")
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
