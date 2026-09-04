import type {
  AIAssistantAccess,
  AIContextUsage,
  AIConversation,
  AIConversationPage,
  AIMessagePart,
  AIModelConfig,
  AIModelOption,
} from './ai-types'
import type { components, operations } from './generated/openapi.js'
import { describe, expectTypeOf, it } from 'vitest'

describe('generated AI transport contracts', () => {
  it('keeps public transport aliases bound to OpenAPI', () => {
    expectTypeOf<AIAssistantAccess>().toEqualTypeOf<operations['getAICapabilities']['responses'][200]['content']['application/json']>()
    expectTypeOf<AIModelOption>().toEqualTypeOf<components['schemas']['AIModelOption']>()
    expectTypeOf<AIModelConfig>().toEqualTypeOf<components['schemas']['AIModelConfig']>()
    expectTypeOf<AIConversation>().toEqualTypeOf<components['schemas']['AIConversation']>()
    expectTypeOf<AIConversationPage>().toEqualTypeOf<operations['listAIConversations']['responses'][200]['content']['application/json']>()
    expectTypeOf<AIMessagePart>().toEqualTypeOf<components['schemas']['AIMessagePart']>()
    expectTypeOf<AIContextUsage>().toEqualTypeOf<components['schemas']['AIContextUsage']>()
  })
})
