import type { OAuthApplication, OAuthApplicationInput, OAuthApplicationSecretResponse, OAuthAuthorizationDecision, OAuthAuthorizationDecisionResponse, OAuthAuthorizationRequest, OAuthDeviceVerification, OAuthDeviceVerificationDecision, OAuthDeviceVerificationResult, OAuthGrant, PaginatedResponse, PaginationParams } from '../types'
import { paginationQuery, request } from '../core'

export const oauthApi = {
  listOAuthApplications: (params: PaginationParams) =>
    request<PaginatedResponse<OAuthApplication>>(`/oauth/applications?${paginationQuery(params)}`),
  createOAuthApplication: (payload: OAuthApplicationInput) =>
    request<OAuthApplicationSecretResponse>('/oauth/applications', { method: 'POST', body: JSON.stringify(payload) }),
  updateOAuthApplication: (applicationId: string, payload: OAuthApplicationInput) =>
    request<OAuthApplication>(`/oauth/applications/${applicationId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  rotateOAuthApplicationSecret: (applicationId: string) =>
    request<OAuthApplicationSecretResponse>(`/oauth/applications/${applicationId}/rotate-secret`, { method: 'POST' }),
  deleteOAuthApplication: (applicationId: string) =>
    request<void>(`/oauth/applications/${applicationId}`, { method: 'DELETE' }),
  listMyOAuthGrants: (params: PaginationParams) =>
    request<PaginatedResponse<OAuthGrant>>(`/oauth/grants?${paginationQuery(params)}`),
  revokeMyOAuthGrant: (grantId: string) =>
    request<void>(`/oauth/grants/${grantId}`, { method: 'DELETE' }),
  getOAuthAuthorizationRequest: (query: string) =>
    request<OAuthAuthorizationRequest>(`/oauth/authorize?${query}`),
  decideOAuthAuthorization: (payload: OAuthAuthorizationDecision) =>
    request<OAuthAuthorizationDecisionResponse>('/oauth/authorize', { method: 'POST', body: JSON.stringify(payload) }),
  getOAuthDeviceVerification: (userCode: string) =>
    request<OAuthDeviceVerification>(`/oauth/device/verification?user_code=${encodeURIComponent(userCode)}`),
  decideOAuthDeviceVerification: (payload: OAuthDeviceVerificationDecision) =>
    request<OAuthDeviceVerificationResult>('/oauth/device/verification', { method: 'POST', body: JSON.stringify(payload) }),
}
