import type { AIModelOption } from '@/api'

export const NEW_CONVERSATION_MODEL_KEY = '__new__'

export function aiConversationModelKey(conversationId?: string): string {
  return conversationId ?? NEW_CONVERSATION_MODEL_KEY
}

export function resolveAIConversationModel(
  models: AIModelOption[],
  conversationId: string | undefined,
  persistedModelId: string | undefined,
  overrides: Record<string, string>,
): AIModelOption | undefined {
  const requestedModelId = overrides[aiConversationModelKey(conversationId)] ?? persistedModelId
  return models.find(model => model.id === requestedModelId) ?? models[0]
}
