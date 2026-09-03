export const RUNTIME_TERMINAL_WEBSOCKET_SUBPROTOCOL = 'luna.devops.terminal.v1'

export interface RuntimeTerminalExitControl {
  type: 'exit'
  code: number
}

const terminalInputEncoder = new TextEncoder()

export function encodeRuntimeTerminalInput(data: string) {
  return terminalInputEncoder.encode(data)
}

export function parseRuntimeTerminalControl(data: string): RuntimeTerminalExitControl | null {
  try {
    const value: unknown = JSON.parse(data)
    if (
      typeof value === 'object'
      && value !== null
      && 'type' in value
      && value.type === 'exit'
      && 'code' in value
      && typeof value.code === 'number'
      && Number.isInteger(value.code)
      && value.code >= 0
      && value.code <= 255
    ) {
      return { type: 'exit', code: value.code }
    }
  }
  catch {
    // Text frames are protocol controls, never terminal output.
  }
  return null
}
