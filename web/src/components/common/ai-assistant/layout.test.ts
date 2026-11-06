import { afterEach, describe, expect, it } from 'vitest'
import {
  AI_ASSISTANT_PANEL_MIN_WIDTH,
  AI_ASSISTANT_PANEL_WIDTH,
  clampAssistantPanelWidth,
  maxAssistantPanelWidth,
  readAssistantPanelWidth,
  saveAssistantPanelWidth,
} from './layout'

afterEach(() => localStorage.clear())

describe('assistant panel width', () => {
  it('keeps room for the workspace and never shrinks below the minimum', () => {
    expect(maxAssistantPanelWidth(1440)).toBe(672)
    expect(maxAssistantPanelWidth(2560)).toBe(1200)
    expect(maxAssistantPanelWidth(1024)).toBe(AI_ASSISTANT_PANEL_MIN_WIDTH)
    expect(clampAssistantPanelWidth(100, 1440)).toBe(AI_ASSISTANT_PANEL_MIN_WIDTH)
    expect(clampAssistantPanelWidth(5000, 1440)).toBe(672)
    expect(clampAssistantPanelWidth(452.4, 1440)).toBe(452)
  })

  it('remembers the dragged width and falls back to the default', () => {
    expect(readAssistantPanelWidth()).toBe(AI_ASSISTANT_PANEL_WIDTH)
    saveAssistantPanelWidth(640.6)
    expect(readAssistantPanelWidth()).toBe(641)
    localStorage.setItem('luna.ai-assistant.panel-width.v1', 'oops')
    expect(readAssistantPanelWidth()).toBe(AI_ASSISTANT_PANEL_WIDTH)
  })
})
