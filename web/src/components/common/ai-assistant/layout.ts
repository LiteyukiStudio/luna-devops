import { safeStorageGet, safeStorageSet } from '@/lib/safe-storage'

export const AI_ASSISTANT_SPLIT_MIN_WIDTH = 720
export const AI_ASSISTANT_SIDEBAR_WIDTH = 264
/** 右侧停靠面板的默认、最小与最大宽度；更大空间由面板头部的展开按钮进入全页助手。 */
export const AI_ASSISTANT_PANEL_WIDTH = 440
export const AI_ASSISTANT_PANEL_MIN_WIDTH = 360
const AI_ASSISTANT_PANEL_MAX_WIDTH = 1200
/**
 * 拖宽面板时给工作区保留的最小宽度：左侧导航 288px + 正文约 480px。
 * 页面布局按工作区容器（`@container/workspace`）而非视口响应，正文变窄时多列自动堆叠，无需为宽屏排版预留空间。
 */
const AI_ASSISTANT_WORKSPACE_RESERVE = 768
export const AI_ASSISTANT_PANEL_KEYBOARD_STEP = 32
const PANEL_WIDTH_STORAGE_KEY = 'luna.ai-assistant.panel-width.v1'

export type AIDesktopConversationLayout = 'overlay' | 'split'

export function resolveAIDesktopConversationLayout(width: number): AIDesktopConversationLayout {
  return width >= AI_ASSISTANT_SPLIT_MIN_WIDTH ? 'split' : 'overlay'
}

export function maxAssistantPanelWidth(viewportWidth: number) {
  return Math.max(AI_ASSISTANT_PANEL_MIN_WIDTH, Math.min(AI_ASSISTANT_PANEL_MAX_WIDTH, viewportWidth - AI_ASSISTANT_WORKSPACE_RESERVE))
}

export function clampAssistantPanelWidth(width: number, viewportWidth: number) {
  return Math.round(Math.min(maxAssistantPanelWidth(viewportWidth), Math.max(AI_ASSISTANT_PANEL_MIN_WIDTH, width)))
}

export function readAssistantPanelWidth() {
  const stored = Number(safeStorageGet(PANEL_WIDTH_STORAGE_KEY))
  return Number.isFinite(stored) && stored > 0 ? stored : AI_ASSISTANT_PANEL_WIDTH
}

export function saveAssistantPanelWidth(width: number) {
  safeStorageSet(PANEL_WIDTH_STORAGE_KEY, String(Math.round(width)))
}
