import type {
  AICapabilities,
  AIConversation,
  AIModelConfig,
  AIModelOption,
  AIPaginatedResponse,
  AITimeline,
  AITurnCreated,
} from '../ai-types'
import { paginationQuery, request } from '../core'

export const aiApi = {
  getAICapabilities: () => request<AICapabilities>('/ai/capabilities'),
  listAIModels: () => request<AIModelOption[]>('/ai/models'),
  listAIModelConfigs: () => request<AIModelConfig[]>('/configs/ai/models'),
  createAIModel: (payload: { name: string, maxContextTokens: number, maxOutputTokens: number, inputCreditsPerMillion: string, outputCreditsPerMillion: string, cachedInputCreditsPerMillion: string, enabled?: boolean }) =>
    request<AIModelConfig>('/configs/ai/models', { method: 'POST', body: JSON.stringify(payload) }),
  updateAIModel: (id: string, payload: Partial<{ name: string, maxContextTokens: number, maxOutputTokens: number, inputCreditsPerMillion: string, outputCreditsPerMillion: string, cachedInputCreditsPerMillion: string, enabled: boolean }>) =>
    request<AIModelConfig>(`/configs/ai/models/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteAIModel: (id: string) =>
    request<void>(`/configs/ai/models/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  listAIConversations: (params: { page: number, pageSize: number, search?: string }) =>
    request<AIPaginatedResponse<AIConversation>>(`/ai/conversations?${paginationQuery({ ...params, sortBy: 'updatedAt', sortOrder: 'desc' })}`),
  createAIConversation: (payload: { modelId: string, projectId?: string, title?: string }) =>
    request<AIConversation>('/ai/conversations', { method: 'POST', body: JSON.stringify(payload) }),
  updateAIConversation: (conversationId: string, payload: { title?: string, modelId?: string }) =>
    request<AIConversation>(`/ai/conversations/${encodeURIComponent(conversationId)}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  renameAIConversation: (conversationId: string, title: string) =>
    aiApi.updateAIConversation(conversationId, { title }),
  deleteAIConversation: (conversationId: string) =>
    request<void>(`/ai/conversations/${encodeURIComponent(conversationId)}`, { method: 'DELETE' }),
  getAIConversationTimeline: (conversationId: string, params: { before?: string, limit?: number } = {}) => {
    const query = new URLSearchParams()
    if (params.before)
      query.set('before', params.before)
    if (params.limit !== undefined)
      query.set('limit', String(params.limit))
    const suffix = query.size > 0 ? `?${query}` : ''
    return request<AITimeline>(`/ai/conversations/${encodeURIComponent(conversationId)}/timeline${suffix}`)
  },
  createAITurn: (conversationId: string, payload: { modelId: string, input: { parts: Array<{ type: 'text', text: string }> }, pageContext: Record<string, unknown> }, idempotencyKey: string) =>
    request<AITurnCreated>(`/ai/conversations/${encodeURIComponent(conversationId)}/turns`, {
      method: 'POST',
      headers: { 'Idempotency-Key': idempotencyKey },
      body: JSON.stringify(payload),
    }),
  executeAIToolAction: (conversationId: string, payload: { operationId: string, arguments: Record<string, unknown>, message: string }, idempotencyKey: string) =>
    request<AITurnCreated>(`/ai/conversations/${encodeURIComponent(conversationId)}/tool-actions`, {
      method: 'POST',
      headers: { 'Idempotency-Key': idempotencyKey },
      body: JSON.stringify(payload),
    }),
  cancelAIRun: (runId: string) =>
    request<void>(`/ai/runs/${encodeURIComponent(runId)}/cancel`, { method: 'POST' }),
  decideAIToolApproval: (runId: string, toolCallId: string, payload: { decision: 'reject' | 'approve' }) =>
    request<void>(`/ai/runs/${encodeURIComponent(runId)}/approvals/${encodeURIComponent(toolCallId)}/decision`, { method: 'POST', body: JSON.stringify(payload) }),
  submitAIRunInput: (runId: string, payload: { input: { parts: Array<{ type: 'text', text: string }> }, expectedVersion: number }) =>
    request<void>(`/ai/runs/${encodeURIComponent(runId)}/input`, { method: 'POST', body: JSON.stringify(payload) }),
}
