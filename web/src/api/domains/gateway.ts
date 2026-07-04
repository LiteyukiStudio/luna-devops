import type { AccessToken, AccessTokenScopeCatalog, GatewayDomainCheckResult, GatewayRoute, PaginatedResponse, PaginationParams } from '../types'
import { paginationQuery, request } from '../core'

type GatewayRoutePayload = Omit<GatewayRoute, 'id' | 'projectId' | 'createdBy' | 'createdAt' | 'cnameName' | 'cnameTarget' | 'accessUrl' | 'routeSummary' | 'conditions' | 'deleteStatus' | 'deleteMessage' | 'deleteStartedAt' | 'deleteFinishedAt'>

export const gatewayApi = {
  listGatewayRoutes: (projectId: string) =>
    request<GatewayRoute[]>(`/projects/${projectId}/gateway-routes`),
  listGatewayRoutesPage: (projectId: string, params: PaginationParams) =>
    request<PaginatedResponse<GatewayRoute>>(`/projects/${projectId}/gateway-routes?${paginationQuery(params)}`),
  createGatewayRoute: (projectId: string, payload: GatewayRoutePayload) =>
    request<GatewayRoute>(`/projects/${projectId}/gateway-routes`, { method: 'POST', body: JSON.stringify(payload) }),
  updateGatewayRoute: (projectId: string, routeId: string, payload: GatewayRoutePayload) =>
    request<GatewayRoute>(`/projects/${projectId}/gateway-routes/${routeId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteGatewayRoute: (projectId: string, routeId: string) =>
    request<void>(`/projects/${projectId}/gateway-routes/${routeId}`, { method: 'DELETE' }),
  checkGatewayDomain: (projectId: string, host: string, routeId?: string) =>
    request<GatewayDomainCheckResult>(`/projects/${projectId}/gateway-routes/check-domain?${new URLSearchParams({ host, ...(routeId ? { routeId } : {}) }).toString()}`),

  listAccessTokens: (params: PaginationParams) =>
    request<PaginatedResponse<AccessToken>>(`/access-tokens?${paginationQuery(params)}`),
  listAccessTokenScopes: () =>
    request<AccessTokenScopeCatalog>('/access-tokens/scopes'),
  createAccessToken: (payload: { name: string, scope: string, expiresInDays: number }) =>
    request<{ token: AccessToken, accessToken: string }>('/access-tokens', { method: 'POST', body: JSON.stringify(payload) }),
  revokeAccessToken: (tokenId: string) =>
    request<AccessToken>(`/access-tokens/${tokenId}`, { method: 'DELETE' }),
}
