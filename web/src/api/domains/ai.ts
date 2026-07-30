import type {
  AICapabilities,
  AIConversation,
  AIMFAResumePayload,
  AIPaginatedResponse,
  AIPendingUIActions,
  AITimeline,
  AITurnCreated,
  AIUIActionAcknowledgement,
} from '../ai-types'
import { paginationQuery, request } from '../core'

export const aiApi = {
  getAICapabilities: () => request<AICapabilities>('/ai/capabilities'),
  listAIConversations: (params: { page: number, pageSize: number, search?: string }) =>
    request<AIPaginatedResponse<AIConversation>>(`/ai/conversations?${paginationQuery({ ...params, sortBy: 'updatedAt', sortOrder: 'desc' })}`),
  createAIConversation: (payload: { projectId?: string, title?: string }) =>
    request<AIConversation>('/ai/conversations', { method: 'POST', body: JSON.stringify(payload) }),
  renameAIConversation: (conversationId: string, title: string) =>
    request<AIConversation>(`/ai/conversations/${encodeURIComponent(conversationId)}`, { method: 'PATCH', body: JSON.stringify({ title }) }),
  deleteAIConversation: (conversationId: string) =>
    request<void>(`/ai/conversations/${encodeURIComponent(conversationId)}`, { method: 'DELETE' }),
  getAIConversationTimeline: (conversationId: string) =>
    request<AITimeline>(`/ai/conversations/${encodeURIComponent(conversationId)}/timeline`),
  createAITurn: (conversationId: string, payload: { input: { parts: Array<{ type: 'text', text: string }> }, pageContext: Record<string, unknown>, clientInstanceId: string }, idempotencyKey: string) =>
    request<AITurnCreated>(`/ai/conversations/${encodeURIComponent(conversationId)}/turns`, {
      method: 'POST',
      headers: { 'Idempotency-Key': idempotencyKey },
      body: JSON.stringify(payload),
    }),
  listPendingAIUIActions: (clientInstanceId: string) =>
    request<AIPendingUIActions>(`/ai/ui-actions/pending?${new URLSearchParams({ clientInstanceId })}`),
  acknowledgeAIUIAction: (actionId: string, payload: AIUIActionAcknowledgement) =>
    request<void>(`/ai/ui-actions/${encodeURIComponent(actionId)}/ack`, {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  cancelAIRun: (runId: string) =>
    request<void>(`/ai/runs/${encodeURIComponent(runId)}/cancel`, { method: 'POST' }),
  decideAIToolApproval: (runId: string, toolCallId: string, payload: { decision: 'approve' | 'reject' | 'approve_all', argumentsHash: string, expectedVersion: number, reason?: string }) =>
    request<void>(`/ai/runs/${encodeURIComponent(runId)}/approvals/${encodeURIComponent(toolCallId)}/decision`, { method: 'POST', body: JSON.stringify(payload) }),
  resumeAIToolMFA: (runId: string, toolCallId: string, payload: AIMFAResumePayload) =>
    request<void>(`/ai/runs/${encodeURIComponent(runId)}/mfa/${encodeURIComponent(toolCallId)}/resume`, { method: 'POST', body: JSON.stringify(payload) }),
  submitAIRunInput: (runId: string, payload: { input: { parts: Array<{ type: 'text', text: string }> }, expectedVersion: number }) =>
    request<void>(`/ai/runs/${encodeURIComponent(runId)}/input`, { method: 'POST', body: JSON.stringify(payload) }),
}
