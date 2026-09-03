import { describe, expect, it } from 'vitest'
import {
  encodeRuntimeTerminalInput,
  parseRuntimeTerminalControl,
  RUNTIME_TERMINAL_WEBSOCKET_SUBPROTOCOL,
} from './runtime-terminal-protocol'

describe('runtime terminal protocol', () => {
  it('encodes terminal input as UTF-8 bytes without changing control bytes', () => {
    expect([...encodeRuntimeTerminalInput('中\u001B[A')]).toEqual([
      0xE4,
      0xB8,
      0xAD,
      0x1B,
      0x5B,
      0x41,
    ])
  })

  it('parses exit controls and never treats arbitrary text as terminal output', () => {
    expect(parseRuntimeTerminalControl('{"type":"exit","code":42}')).toEqual({ type: 'exit', code: 42 })
    expect(parseRuntimeTerminalControl('plain terminal text')).toBeNull()
    expect(parseRuntimeTerminalControl('{"type":"resize","cols":80,"rows":24}')).toBeNull()
  })

  it('uses the versioned WebSocket subprotocol', () => {
    expect(RUNTIME_TERMINAL_WEBSOCKET_SUBPROTOCOL).toBe('luna.devops.terminal.v1')
  })
})
