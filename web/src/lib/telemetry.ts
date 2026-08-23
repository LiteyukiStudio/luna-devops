import type { Span, SpanOptions } from '@opentelemetry/api'
import { context, propagation, SpanKind, SpanStatusCode, trace } from '@opentelemetry/api'

const tracerName = 'luna-web'
const dynamicResourceParents = new Set([
  'applications',
  'build-jobs',
  'build-runs',
  'clusters',
  'conversations',
  'deployment-targets',
  'environments',
  'external-identities',
  'oidc',
  'approvals',
  'providers',
  'projects',
  'registries',
  'releases',
  'routes',
  'runs',
  'tokens',
  'turns',
  'ui-actions',
  'users',
  'volume-imports',
  'volume-transfers',
  'volumes',
])

let telemetryRuntimePromise: Promise<void> | undefined

export function enableBrowserTelemetry() {
  if (telemetryRuntimePromise || typeof window === 'undefined')
    return

  const runtimePromise = import('./browser-telemetry-runtime').then(({ enableBrowserTelemetryRuntime }) => {
    enableBrowserTelemetryRuntime()
  })
  telemetryRuntimePromise = runtimePromise
  void runtimePromise.catch(() => {
    if (telemetryRuntimePromise === runtimePromise)
      telemetryRuntimePromise = undefined
  })
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

export function startUserOperation(operation: string) {
  return startSpan(operation)
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
  if (isStableErrorCode(error))
    span.setAttribute('error.code', error.code)
}

function isStableErrorCode(error: unknown): error is { code: string } {
  return typeof error === 'object'
    && error !== null
    && 'code' in error
    && typeof error.code === 'string'
    && /^[a-z][a-z0-9_.-]{2,120}$/.test(error.code)
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
    if (/^(?:prj|app|rel|run|usr|reg|clu|dpt|env|pvol|vtx|aicnv|aiturn|airun|aiitem|aitc|aiuia)_[\w-]+$/i.test(segment))
      return ':id'
    if (/^[0-9a-f]{8}-[0-9a-f-]{27,}$/i.test(segment))
      return ':id'
    return segment
  }).join('/')}`
}
