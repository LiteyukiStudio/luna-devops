import type { BillingListParams, BuildRunListParams, PaginationParams, RuntimeClusterResourceListParams } from './types'
import i18next from '@/i18n'

export const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

interface ApiErrorBody {
  code?: unknown
  detail?: unknown
  error?: unknown
}

export class ApiError extends Error {
  code: string
  detail?: string
  path: string
  status: number

  constructor(message: string, options: { code?: string, detail?: string, path: string, status: number }) {
    super(message)
    this.name = 'ApiError'
    this.code = options.code || 'request.failed'
    this.detail = options.detail
    this.path = options.path
    this.status = options.status
  }
}

export function optionalProjectQuery(projectId?: unknown) {
  if (typeof projectId !== 'string')
    return ''
  const normalized = projectId.trim()
  return normalized ? `?projectId=${encodeURIComponent(normalized)}` : ''
}

export function paginationQuery(params: PaginationParams & { scope?: string }) {
  const search = new URLSearchParams({
    page: String(params.page),
    pageSize: String(params.pageSize),
  })
  if (params.sortBy)
    search.set('sortBy', params.sortBy)
  if (params.sortOrder)
    search.set('sortOrder', params.sortOrder)
  if (params.search)
    search.set('search', params.search)
  if (params.scope)
    search.set('scope', params.scope)
  return search.toString()
}

export function paginationWithProjectQuery(params: PaginationParams & { projectId?: string }) {
  const search = new URLSearchParams(paginationQuery(params))
  if (params.projectId)
    search.set('projectId', params.projectId)
  return search.toString()
}

export function buildRunListQuery(params: BuildRunListParams) {
  const search = new URLSearchParams(paginationQuery(params))
  if (params.applicationId)
    search.set('applicationId', params.applicationId)
  if (params.deploymentTargetId)
    search.set('deploymentTargetId', params.deploymentTargetId)
  if (params.status)
    search.set('status', params.status)
  if (params.triggerType)
    search.set('triggerType', params.triggerType)
  if (params.sourceBranch)
    search.set('sourceBranch', params.sourceBranch)
  if (params.createdBy)
    search.set('createdBy', params.createdBy)
  return search.toString()
}

export function runtimeClusterResourceListQuery(params: RuntimeClusterResourceListParams) {
  const search = new URLSearchParams(paginationQuery(params))
  search.set('kind', params.kind)
  if (params.namespace)
    search.set('namespace', params.namespace)
  if (params.projectId)
    search.set('projectId', params.projectId)
  if (params.applicationId)
    search.set('applicationId', params.applicationId)
  if (params.environmentId)
    search.set('environmentId', params.environmentId)
  return search.toString()
}

export function billingQuery(params: BillingListParams) {
  const search = new URLSearchParams(paginationQuery(params))
  for (const projectId of params.projectIds ?? []) {
    if (projectId)
      search.append('projectIds', projectId)
  }
  if (params.type)
    search.set('type', params.type)
  if (params.meter)
    search.set('meter', params.meter)
  if (params.periodStart)
    search.set('periodStart', params.periodStart)
  if (params.periodEnd)
    search.set('periodEnd', params.periodEnd)
  return search.toString()
}

export function billingSummaryQuery(projectIds?: string[], period?: { periodStart?: string, periodEnd?: string, accountScope?: string }) {
  const search = new URLSearchParams()
  for (const projectId of projectIds ?? []) {
    if (projectId)
      search.append('projectIds', projectId)
  }
  if (period?.periodStart)
    search.set('periodStart', period.periodStart)
  if (period?.periodEnd)
    search.set('periodEnd', period.periodEnd)
  if (period?.accountScope)
    search.set('accountScope', period.accountScope)
  const query = search.toString()
  return query ? `?${query}` : ''
}

function translatedErrorMessage(code: string) {
  const translationKey = code ? `errors.${code}` : ''
  return translationKey && i18next.exists(translationKey) ? i18next.t(translationKey) : ''
}

function fallbackMessageForStatus(status: number) {
  if (status === 401)
    return translatedErrorMessage('auth.unauthorized')
  if (status === 403)
    return translatedErrorMessage('auth.forbidden')
  if (status === 404)
    return translatedErrorMessage('resource.not_found')
  if (status === 409)
    return translatedErrorMessage('resource.conflict')
  if (status === 429)
    return translatedErrorMessage('rate_limited')
  if (status >= 500)
    return translatedErrorMessage('internal_error')
  return translatedErrorMessage('request.failed')
}

async function parseErrorBody(response: Response): Promise<ApiErrorBody> {
  const contentType = response.headers.get('content-type') ?? ''
  if (contentType.includes('application/json')) {
    return response.json().catch(() => ({}))
  }
  const text = await response.text().catch(() => '')
  return text.trim() ? { error: text.trim() } : {}
}

async function apiErrorFromResponse(response: Response, path: string) {
  const body = await parseErrorBody(response)
  const code = typeof body.code === 'string' && body.code.trim() ? body.code.trim() : ''
  const detail = typeof body.detail === 'string' && body.detail.trim() ? body.detail.trim() : ''
  const bodyError = typeof body.error === 'string' && body.error.trim() ? body.error.trim() : ''
  const message = translatedErrorMessage(code) || detail || bodyError || fallbackMessageForStatus(response.status) || response.statusText
  return new ApiError(message, {
    code: code || `http.${response.status}`,
    detail: detail || bodyError,
    path,
    status: response.status,
  })
}

function apiNetworkError(path: string, error: unknown) {
  const detail = error instanceof Error ? error.message : String(error)
  const message = translatedErrorMessage('network.failed') || detail
  return new ApiError(message, {
    code: 'network.failed',
    detail,
    path,
    status: 0,
  })
}

export async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const { headers, ...requestOptions } = options ?? {}
  let response: Response
  try {
    response = await fetch(`${API_BASE_URL}${path}`, {
      ...requestOptions,
      credentials: 'include',
      headers: {
        'Accept-Language': i18next.language,
        'Content-Type': 'application/json',
        ...headers,
      },
    })
  }
  catch (error) {
    throw apiNetworkError(path, error)
  }

  if (!response.ok) {
    throw await apiErrorFromResponse(response, path)
  }

  if (response.status === 204)
    return undefined as T

  return response.json()
}
