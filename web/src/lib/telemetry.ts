import type { Attributes, Span, SpanOptions } from '@opentelemetry/api'
import type { BatchSpanProcessor } from '@opentelemetry/sdk-trace-web'
import { context, propagation, SpanKind, SpanStatusCode, trace } from '@opentelemetry/api'
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-proto'
import { resourceFromAttributes } from '@opentelemetry/resources'
import { BatchSpanProcessor as WebBatchSpanProcessor, WebTracerProvider } from '@opentelemetry/sdk-trace-web'
import { ATTR_SERVICE_NAME } from '@opentelemetry/semantic-conventions'

const tracerName = 'luna-web'
const dynamicResourceParents = new Set([
  'applications',
  'build-jobs',
  'build-runs',
  'clusters',
  'deployment-targets',
  'environments',
  'external-identities',
  'oidc',
  'providers',
  'projects',
  'registries',
  'releases',
  'routes',
  'tokens',
  'users',
  'volume-imports',
  'volume-transfers',
  'volumes',
])

let provider: WebTracerProvider | undefined
let spanProcessor: BatchSpanProcessor | undefined

export function enableBrowserTelemetry() {
  if (provider || typeof window === 'undefined')
    return

  const exporter = createBrowserTraceExporter()
  spanProcessor = new WebBatchSpanProcessor(exporter, {
    maxExportBatchSize: 64,
    maxQueueSize: 256,
    scheduledDelayMillis: 3_000,
  })
  provider = new WebTracerProvider({
    resource: resourceFromAttributes({
      [ATTR_SERVICE_NAME]: 'luna-web',
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

export function startAPIRequestSpan(method: string, path: string) {
  const route = normalizeRoute(path)
  const span = startSpan(`${method.toUpperCase()} ${route}`, {
    kind: SpanKind.CLIENT,
    attributes: {
      'http.request.method': method.toUpperCase(),
      'http.route': route,
      'server.address': window.location.hostname,
    },
  })
  const spanContext = trace.setSpan(context.active(), span)
  const headers: Record<string, string> = {}
  propagation.inject(spanContext, headers)

  return {
    headers,
    finish(response: Response) {
      span.setAttribute('http.response.status_code', response.status)
      if (!response.ok)
        span.setStatus({ code: SpanStatusCode.ERROR, message: `HTTP ${response.status}` })
      span.end()
    },
    fail(error: unknown) {
      recordSpanError(span, error)
      span.end()
    },
  }
}

export function recordNavigation(path: string) {
  const span = startSpan('navigation.change', {
    attributes: { 'navigation.route': normalizeRoute(path) },
  })
  span.end()
}

export function createTracedEventSource(url: string, options: EventSourceInit, operation: string) {
  const span = startSpan(operation, {
    kind: SpanKind.CLIENT,
    attributes: {
      'network.protocol.name': 'sse',
      'url.template': normalizeRoute(url),
    },
  })
  const source = new EventSource(withStreamTraceContext(url, span), options)
  let ended = false

  source.addEventListener('open', () => span.addEvent('stream.open'))
  source.addEventListener('error', () => {
    span.addEvent('stream.error', { ready_state: source.readyState })
    if (source.readyState === EventSource.CLOSED)
      endSpan()
  })

  const originalClose = source.close.bind(source)
  source.close = () => {
    originalClose()
    span.addEvent('stream.closed')
    endSpan()
  }
  return source

  function endSpan() {
    if (ended)
      return
    ended = true
    span.end()
  }
}

export function createTracedWebSocket(url: string, protocols: string | string[] | undefined, operation: string) {
  const span = startSpan(operation, {
    kind: SpanKind.CLIENT,
    attributes: {
      'network.protocol.name': 'websocket',
      'url.template': normalizeRoute(url),
    },
  })
  const tracedURL = withStreamTraceContext(url, span)
  const socket = protocols === undefined ? new WebSocket(tracedURL) : new WebSocket(tracedURL, protocols)
  let ended = false
  socket.addEventListener('open', () => span.addEvent('socket.open'))
  socket.addEventListener('error', () => {
    span.setStatus({ code: SpanStatusCode.ERROR, message: 'WebSocket error' })
    span.addEvent('socket.error')
  })
  socket.addEventListener('close', (event) => {
    span.setAttribute('websocket.close.code', event.code)
    if (!event.wasClean)
      span.setStatus({ code: SpanStatusCode.ERROR, message: 'WebSocket closed unexpectedly' })
    if (!ended) {
      ended = true
      span.end()
    }
  })
  return socket
}

function withStreamTraceContext(url: string, span: Span) {
  const carrier: Record<string, string> = {}
  propagation.inject(trace.setSpan(context.active(), span), carrier)
  const parsed = new URL(url, window.location.origin)
  if (carrier.traceparent)
    parsed.searchParams.set('_otel_traceparent', carrier.traceparent)
  if (carrier.tracestate)
    parsed.searchParams.set('_otel_tracestate', carrier.tracestate)
  return parsed.toString()
}

export function startUserOperation(operation: string, attributes?: Attributes) {
  return startSpan(operation, { attributes })
}

export function recordInteractionCardRenderError(scope: 'group' | 'card' | 'content' | 'field' | 'action', error: unknown) {
  const span = startSpan('ai.interaction_card.render', {
    attributes: {
      'ai.interaction_card.render.scope': scope,
    },
  })
  recordSpanError(span, error)
  span.end()
}

function startSpan(name: string, options?: SpanOptions) {
  return trace.getTracer(tracerName).startSpan(name, options)
}

function recordSpanError(span: Span, error: unknown) {
  const normalized = error instanceof Error ? error : new Error(String(error))
  span.setStatus({ code: SpanStatusCode.ERROR, message: normalized.name })
  span.setAttribute('error.type', normalized.name)
}

export function normalizeTelemetryRoute(input: string) {
  return normalizeRoute(input)
}

function normalizeRoute(input: string) {
  let pathname = input
  try {
    pathname = new URL(input, window.location.origin).pathname
  }
  catch {
    pathname = input.split('?')[0]
  }
  const segments = pathname.split('/').filter(Boolean)
  return `/${segments.map((segment, index) => {
    if (index > 0 && dynamicResourceParents.has(segments[index - 1]))
      return ':id'
    if (/^(?:prj|app|rel|run|usr|reg|clu|dpt|env|pvol|vtx)_[\w-]+$/i.test(segment))
      return ':id'
    if (/^[0-9a-f]{8}-[0-9a-f-]{27,}$/i.test(segment))
      return ':id'
    return segment
  }).join('/')}`
}

function browserTraceRelayURL() {
  const apiBase = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'
  return `${apiBase.replace(/\/$/, '')}/telemetry/v1/traces`
}

function flushBrowserTelemetry() {
  void spanProcessor?.forceFlush().catch(() => undefined)
}

function recordWindowError(event: ErrorEvent) {
  const span = startSpan('browser.error', {
    attributes: {
      'error.type': event.error instanceof Error ? event.error.name : 'ErrorEvent',
    },
  })
  span.setStatus({ code: SpanStatusCode.ERROR, message: 'Unhandled browser error' })
  span.end()
}

function recordUnhandledRejection(event: PromiseRejectionEvent) {
  const span = startSpan('browser.unhandled_rejection', {
    attributes: {
      'error.type': event.reason instanceof Error ? event.reason.name : typeof event.reason,
    },
  })
  span.setStatus({ code: SpanStatusCode.ERROR, message: 'Unhandled promise rejection' })
  span.end()
}
