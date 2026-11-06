import type { AIToolStatus } from '@/api'
import type { StatusIndicatorState } from '@/components/common/status-tone'

export type AIStepStatus = 'pending' | 'running' | 'success' | 'warning' | 'error' | 'skipped'

/** 交互卡片状态列表与实时进度步骤的状态到指示器图形。 */
export const aiStepIndicatorStates: Record<AIStepStatus, StatusIndicatorState> = {
  pending: 'queued',
  running: 'running',
  success: 'success',
  warning: 'warning',
  error: 'failure',
  skipped: 'canceled',
}

const aiToolIndicatorStates: Record<AIToolStatus, StatusIndicatorState> = {
  proposed: 'queued',
  awaiting_approval: 'warning',
  running: 'running',
  succeeded: 'success',
  failed: 'failure',
  rejected: 'canceled',
  canceled: 'canceled',
  skipped: 'neutral',
}

/** AI 工具调用状态到指示器图形；运维回读的状态是开放字符串，未知值按中性处理。 */
export function aiToolIndicatorState(status: string): StatusIndicatorState {
  return isAIToolStatus(status) ? aiToolIndicatorStates[status] : 'neutral'
}

function isAIToolStatus(status: string): status is AIToolStatus {
  return Object.hasOwn(aiToolIndicatorStates, status)
}
