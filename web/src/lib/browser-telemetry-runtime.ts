import type { Span } from '@opentelemetry/api'
import type { BatchSpanProcessor } from '@opentelemetry/sdk-trace-web'
import { SpanStatusCode, trace } from '@opentelemetry/api'
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-proto'
import { resourceFromAttributes } from '@opentelemetry/resources'
import { BatchSpanProcessor as WebBatchSpanProcessor, WebTracerProvider } from '@opentelemetry/sdk-trace-web'

const tracerName = 'luna-web'

let provider: WebTracerProvider | undefined
let spanProcessor: BatchSpanProcessor | undefined

export function enableBrowserTelemetryRuntime() {
  if (provider)
    return

  spanProcessor = new WebBatchSpanProcessor(createBrowserTraceExporter(), {
    maxExportBatchSize: 64,
    maxQueueSize: 256,
    scheduledDelayMillis: 3_000,
  })
  provider = new WebTracerProvider({
    resource: resourceFromAttributes({
      'service.name': 'luna-web',
      'service.version': import.meta.env.VITE_APP_COMMIT_SHA || 'dev',
      'deployment.environment.name': import.meta.env.MODE,
    }),
    spanProcessors: [spanProcessor],
  })
  provider.register()
  window.addEventListener('pagehide', flushBrowserTelemetry, { capture: true })
  window.addEventListener('error', recordWindowError)
  window.addEventListener('unhandledrejection', recordUnhandledRejection)
}

export function createBrowserTraceExporter(url = browserTraceRelayURL()) {
  return new OTLPTraceExporter({ url })
}

function browserTraceRelayURL() {
  const apiBase = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'
  return `${apiBase.replace(/\/$/, '')}/telemetry/v1/traces`
}

function flushBrowserTelemetry() {
  void spanProcessor?.forceFlush().catch(() => undefined)
}

function recordWindowError(event: ErrorEvent) {
  const span = startSpan('browser.error')
  span.setAttribute('error.type', event.error instanceof Error ? event.error.name : 'ErrorEvent')
  span.setStatus({ code: SpanStatusCode.ERROR, message: 'Unhandled browser error' })
  span.end()
}

function recordUnhandledRejection(event: PromiseRejectionEvent) {
  const span = startSpan('browser.unhandled_rejection')
  span.setAttribute('error.type', event.reason instanceof Error ? event.reason.name : typeof event.reason)
  span.setStatus({ code: SpanStatusCode.ERROR, message: 'Unhandled promise rejection' })
  span.end()
}

function startSpan(name: string): Span {
  return trace.getTracer(tracerName).startSpan(name)
}
