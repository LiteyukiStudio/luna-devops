import type {
  CreateKubeCredentialInput,
  CreateKubeCredentialResponse,
  KubeCredential,
  KubeCredentialBinding,
  PaginatedResponse,
  PaginationParams,
  RuntimeClusterKubeGateway,
  RuntimeClusterKubeGatewayRule,
  UpdateRuntimeClusterKubeGatewayInput,
} from '../types'
import { paginationQuery, request } from '../core'

type RuntimeClusterKubeGatewayWire = Omit<RuntimeClusterKubeGateway, 'extraResourceRules'> & {
  extraResourceRules?: Array<Omit<RuntimeClusterKubeGatewayRule, 'subresources'> & {
    subresources?: string[] | null
  }> | null
}

function normalizeRuntimeClusterKubeGateway(response: RuntimeClusterKubeGatewayWire): RuntimeClusterKubeGateway {
  const rules = Array.isArray(response.extraResourceRules) ? response.extraResourceRules : []
  return {
    ...response,
    extraResourceRules: rules.map(rule => ({
      ...rule,
      subresources: Array.isArray(rule.subresources) ? rule.subresources : [],
    })),
  }
}

export const kubectlApi = {
  createKubeCredential: (payload: CreateKubeCredentialInput) =>
    request<CreateKubeCredentialResponse>('/kube-credentials', { method: 'POST', body: JSON.stringify(payload), cache: 'no-store' }),
  listKubeCredentials: (params: PaginationParams & { status?: string }) => {
    const search = new URLSearchParams(paginationQuery(params))
    if (params.status)
      search.set('status', params.status)
    return request<PaginatedResponse<KubeCredential>>(`/kube-credentials?${search.toString()}`)
  },
  listKubeCredentialBindings: (credentialId: string, params: PaginationParams) =>
    request<PaginatedResponse<KubeCredentialBinding>>(`/kube-credentials/${encodeURIComponent(credentialId)}/bindings?${paginationQuery(params)}`),
  revokeKubeCredential: (credentialId: string) =>
    request<void>(`/kube-credentials/${encodeURIComponent(credentialId)}`, { method: 'DELETE' }),
  getRuntimeClusterKubeGateway: (clusterId: string) =>
    request<RuntimeClusterKubeGatewayWire>(`/runtime/clusters/${encodeURIComponent(clusterId)}/kube-gateway`, { cache: 'no-store' })
      .then(normalizeRuntimeClusterKubeGateway),
  updateRuntimeClusterKubeGateway: (clusterId: string, payload: UpdateRuntimeClusterKubeGatewayInput) =>
    request<RuntimeClusterKubeGatewayWire>(`/runtime/clusters/${encodeURIComponent(clusterId)}/kube-gateway`, { method: 'PUT', body: JSON.stringify(payload) })
      .then(normalizeRuntimeClusterKubeGateway),
}
