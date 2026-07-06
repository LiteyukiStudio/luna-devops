import { API_BASE_URL } from './core'

export function oidcStartUrl(providerId: string, mode: 'login' | 'bind', redirect = '/projects') {
  const params = new URLSearchParams({ mode, redirect })
  return `${API_BASE_URL}/auth/oidc/${providerId}/start?${params.toString()}`
}

export function apiBaseOrigin() {
  if (!API_BASE_URL.startsWith('http://') && !API_BASE_URL.startsWith('https://')) {
    return window.location.origin
  }
  try {
    return new URL(API_BASE_URL).origin
  }
  catch {
    return window.location.origin
  }
}

export function buildJobLogsStreamUrl(projectId: string, jobId: string, after = 0) {
  const query = new URLSearchParams({ after: String(Math.max(0, after)) })
  return `${API_BASE_URL}/projects/${encodeURIComponent(projectId)}/build-jobs/${encodeURIComponent(jobId)}/logs/stream?${query.toString()}`
}

export function deploymentTargetDataExportUrl(projectId: string, applicationId: string, targetId: string) {
  return `${API_BASE_URL}/projects/${encodeURIComponent(projectId)}/applications/${encodeURIComponent(applicationId)}/deployment-targets/${encodeURIComponent(targetId)}/data-export`
}

export function deploymentTargetMetricsStreamUrl(projectId: string, applicationId: string, targetId: string) {
  return `${API_BASE_URL}/projects/${encodeURIComponent(projectId)}/applications/${encodeURIComponent(applicationId)}/deployment-targets/${encodeURIComponent(targetId)}/metrics/stream`
}

function apiWebSocketUrl(path: string) {
  const base = API_BASE_URL.startsWith('http://') || API_BASE_URL.startsWith('https://')
    ? API_BASE_URL
    : `${window.location.origin}${API_BASE_URL.startsWith('/') ? '' : '/'}${API_BASE_URL}`
  const url = new URL(`${base}${path}`)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return url
}

export function releaseRuntimeTerminalUrl(projectId: string, releaseId: string, container = '') {
  const url = apiWebSocketUrl(`/projects/${encodeURIComponent(projectId)}/releases/${encodeURIComponent(releaseId)}/terminal`)
  if (container.trim())
    url.searchParams.set('container', container.trim())
  return url.toString()
}

export function runtimeClusterPodTerminalUrl(clusterId: string, namespace: string, podName: string, container = '') {
  const url = apiWebSocketUrl(`/runtime/clusters/${encodeURIComponent(clusterId)}/pods/terminal`)
  url.searchParams.set('namespace', namespace)
  url.searchParams.set('name', podName)
  if (container.trim())
    url.searchParams.set('container', container.trim())
  return url.toString()
}

export function gitOAuthStartUrl(providerId: string, redirect = '/projects', frontendOrigin = window.location.origin, callbackOrigin = apiBaseOrigin()) {
  const params = new URLSearchParams({ callbackOrigin, frontendOrigin, redirect })
  return `${API_BASE_URL}/git/providers/${providerId}/oauth/start?${params.toString()}`
}
