export const AI_ASSISTANT_SPLIT_MIN_WIDTH = 720
export const AI_ASSISTANT_SIDEBAR_WIDTH = 264
/** 右侧停靠面板宽度；需要更大空间时由面板头部的展开按钮进入全页助手。 */
export const AI_ASSISTANT_PANEL_WIDTH = 440

export type AIDesktopConversationLayout = 'overlay' | 'split'

export function resolveAIDesktopConversationLayout(width: number): AIDesktopConversationLayout {
  return width >= AI_ASSISTANT_SPLIT_MIN_WIDTH ? 'split' : 'overlay'
}
