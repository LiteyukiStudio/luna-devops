import { useState } from 'react'

export type TerminalConnectionState = 'connected' | 'connecting' | 'disconnected' | 'failed' | 'idle'

/**
 * Web 终端连接状态由终端面板通过 onStateChange 上报，工具栏据此显示状态与重连。
 * attempt 需拼进终端面板的 key：递增即重建 WebSocket。
 */
export function useTerminalConnection() {
  const [state, setState] = useState<TerminalConnectionState>('connecting')
  const [attempt, setAttempt] = useState(0)

  return {
    attempt,
    state,
    onStateChange: setState,
    reconnect: () => {
      setState('connecting')
      setAttempt(value => value + 1)
    },
    reset: () => setState('connecting'),
  }
}
