import type { BillingListParams, BuildRunListParams, MFAChallenge, MFAPurpose, PaginationParams, RuntimeClusterResourceListParams } from './types'
import i18next from '@/i18n'
import { mfaPurposes } from './types'

export const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

interface ApiErrorBody {
  code?: unknown
  detail?: unknown
  error?: unknown
  purpose?: unknown
}

type MFAChallengeHandler = (challenge: MFAChallenge) => Promise<void>

let mfaChallengeHandler: MFAChallengeHandler | undefined
let mfaChallengeQueue = Promise.resolve()
const activeMfaChallenges = new Map<string, Promise<void>>()

export class ApiError extends Error {
  code: string
  detail?: string
  path: string
  purpose?: MFAPurpose
  status: number

  constructor(message: string, options: { code?: string, detail?: string, path: string, purpose?: MFAPurpose, status: number }) {
    super(message)
    this.name = 'ApiError'
    this.code = options.code || 'request.failed'
    this.detail = options.detail
    this.path = options.path
    this.purpose = options.purpose
    this.status = options.status
  }
}

export function registerMFAChallengeHandler(handler: MFAChallengeHandler) {
  mfaChallengeHandler = handler
  return () => {
    if (mfaChallengeHandler === handler)
      mfaChallengeHandler = undefined
  }
}

function resolveMFAChallenge(challenge: MFAChallenge) {
  const key = challenge.purpose
  const activeChallenge = activeMfaChallenges.get(key)
  if (activeChallenge)
    return activeChallenge

  const handler = mfaChallengeHandler
  if (!handler)
    return Promise.reject(new Error('mfa_challenge_handler_unavailable'))

  const queuedChallenge = mfaChallengeQueue
    .catch(() => undefined)
    .then(() => handler(challenge))
    .then(() => undefined)
  mfaChallengeQueue = queuedChallenge
  activeMfaChallenges.set(key, queuedChallenge)
  void queuedChallenge.then(
    () => activeMfaChallenges.delete(key),
    () => activeMfaChallenges.delete(key),
  )
  return queuedChallenge
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
  if (params.userId)
    search.set('userId', params.userId)
  return search.toString()
}

export function billingSummaryQuery(projectIds?: string[], period?: { periodStart?: string, periodEnd?: string, accountScope?: string, userId?: string }) {
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
  if (period?.userId)
    search.set('userId', period.userId)
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
  const purpose = typeof body.purpose === 'string' && mfaPurposes.includes(body.purpose.trim() as MFAPurpose)
    ? body.purpose.trim() as MFAPurpose
    : undefined
  const message = translatedErrorMessage(code) || detail || bodyError || fallbackMessageForStatus(response.status) || response.statusText
  return new ApiError(message, {
    code: code || `http.${response.status}`,
    detail: detail || bodyError,
    path,
    purpose,
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

async function requestOnce<T>(path: string, options?: RequestInit): Promise<T> {
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

export async function request<T>(path: string, options?: RequestInit): Promise<T> {
  try {
    return await requestOnce<T>(path, options)
  }
  catch (error) {
    if (!(error instanceof ApiError) || error.code !== 'mfa_required' || !error.purpose || path === '/auth/mfa/verify' || !mfaChallengeHandler)
      throw error

    try {
      await resolveMFAChallenge({ purpose: error.purpose })
    }
    catch {
      throw error
    }

    return requestOnce<T>(path, options)
  }
}
