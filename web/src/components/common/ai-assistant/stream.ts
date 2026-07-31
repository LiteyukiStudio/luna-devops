import { createTracedEventSource } from '@/lib/telemetry'

export function createAIEventSource(rawUrl: string, after: number, origin = window.location.origin): EventSource {
  const url = new URL(rawUrl, origin)
  url.searchParams.set('after', String(after))
  url.searchParams.set('stream', 'true')
  return createTracedEventSource(url.toString(), { withCredentials: true }, 'ai.run.events.stream')
}
