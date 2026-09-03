import { act, render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { RuntimeTerminalPanel } from './runtime-terminal-panel'
import { RUNTIME_TERMINAL_WEBSOCKET_SUBPROTOCOL } from './runtime-terminal-protocol'

const mocks = vi.hoisted(() => ({
  createSocket: vi.fn(),
  dataHandler: undefined as ((data: string) => void) | undefined,
  socketClose: vi.fn(),
  socketSend: vi.fn(),
  terminalOptions: {} as { disableStdin?: boolean },
  terminalWrite: vi.fn(),
  terminalWriteln: vi.fn(),
}))

vi.mock('@xterm/addon-fit', () => ({
  FitAddon: class {
    fit() {}
  },
}))

vi.mock('@xterm/xterm', () => ({
  Terminal: class {
    cols = 80
    rows = 24
    options = mocks.terminalOptions

    dispose() {}
    focus() {}
    loadAddon() {}
    open() {}
    write = mocks.terminalWrite
    writeln = mocks.terminalWriteln

    onData(handler: (data: string) => void) {
      mocks.dataHandler = handler
      return { dispose() {} }
    }
  },
}))

vi.mock('@/lib/telemetry', () => ({
  createTracedWebSocket: mocks.createSocket,
}))

class TestSocket extends EventTarget {
  binaryType: BinaryType = 'blob'
  readyState = WebSocket.OPEN
  close = mocks.socketClose
  send = mocks.socketSend
}

describe('runtime terminal panel protocol', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.dataHandler = undefined
    mocks.terminalOptions = {}
  })

  it('negotiates the protocol and sends xterm input as UTF-8 binary data', () => {
    const socket = new TestSocket()
    mocks.createSocket.mockReturnValue(socket)
    render(
      <RuntimeTerminalPanel
        container=""
        projectId="prj_test"
        ready
        release={null}
        socketUrl="ws://terminal.test"
      />,
    )

    expect(mocks.createSocket).toHaveBeenCalledWith(
      'ws://terminal.test',
      RUNTIME_TERMINAL_WEBSOCKET_SUBPROTOCOL,
      'runtime.terminal.websocket',
    )
    act(() => mocks.dataHandler?.('中\u001B[A'))
    expect(mocks.socketSend).toHaveBeenCalledOnce()
    expect([...mocks.socketSend.mock.calls[0][0]]).toEqual([0xE4, 0xB8, 0xAD, 0x1B, 0x5B, 0x41])
  })

  it('consumes exit controls without rendering them and rejects unknown text controls', () => {
    const socket = new TestSocket()
    mocks.createSocket.mockReturnValue(socket)
    render(
      <RuntimeTerminalPanel
        container=""
        projectId="prj_test"
        ready
        release={null}
        socketUrl="ws://terminal.test"
      />,
    )

    act(() => socket.dispatchEvent(new MessageEvent('message', { data: '{"type":"exit","code":7}' })))
    expect(mocks.terminalOptions.disableStdin).toBe(true)
    expect(mocks.terminalWrite).not.toHaveBeenCalled()
    expect(mocks.socketClose).not.toHaveBeenCalled()

    act(() => socket.dispatchEvent(new MessageEvent('message', { data: 'not a terminal control' })))
    expect(mocks.terminalWrite).not.toHaveBeenCalled()
    expect(mocks.socketClose).toHaveBeenCalledWith(1002, 'invalid terminal control message')
  })
})
