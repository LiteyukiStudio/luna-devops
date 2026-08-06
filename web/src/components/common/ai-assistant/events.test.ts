import { describe, expect, it } from 'vitest'
import { AI_ASSISTANT_OPEN_EVENT } from './events'

describe('AI assistant open event', () => {
  it('uses a stable namespaced event name', () => {
    expect(AI_ASSISTANT_OPEN_EVENT).toBe('luna-devops:open-ai-assistant')
  })

  it('delivers the open request to window listeners', () => {
    let received = 0
    const listener = () => {
      received += 1
    }
    window.addEventListener(AI_ASSISTANT_OPEN_EVENT, listener)
    window.dispatchEvent(new CustomEvent(AI_ASSISTANT_OPEN_EVENT))
    window.removeEventListener(AI_ASSISTANT_OPEN_EVENT, listener)
    expect(received).toBe(1)
  })
})
