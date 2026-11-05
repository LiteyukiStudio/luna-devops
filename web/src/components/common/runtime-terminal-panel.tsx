import type { ITheme } from '@xterm/xterm'
import type { TerminalConnectionState } from './use-terminal-connection'
import type { Release } from '@/api'
import { FitAddon } from '@xterm/addon-fit'
import { Terminal as XTerm } from '@xterm/xterm'
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { releaseRuntimeTerminalUrl } from '@/api'
import { createTracedWebSocket } from '@/lib/telemetry'
import { cn } from '@/lib/utils'
import {
  encodeRuntimeTerminalInput,
  parseRuntimeTerminalControl,
  RUNTIME_TERMINAL_WEBSOCKET_SUBPROTOCOL,
} from './runtime-terminal-protocol'
import '@xterm/xterm/css/xterm.css'

export function RuntimeTerminalPanel({
  container,
  fullscreen = false,
  pod = '',
  projectId,
  release,
  ready,
  socketUrl,
  onConnectionChange,
}: {
  container: string
  fullscreen?: boolean
  pod?: string
  projectId: string
  ready?: boolean
  release: Release | null
  socketUrl?: string
  /** 必须是稳定引用（例如 useState 的 setter），否则会触发重连。 */
  onConnectionChange?: (state: TerminalConnectionState) => void
}) {
  const { t } = useTranslation()
  const terminalRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    const terminalSocketUrl = socketUrl ?? (release && projectId ? releaseRuntimeTerminalUrl(projectId, release.id, container, pod) : '')
    const shouldConnect = ready ?? Boolean(release && projectId)
    if (!shouldConnect || !terminalSocketUrl || !terminalRef.current) {
      onConnectionChange?.('idle')
      return
    }
    onConnectionChange?.('connecting')

    const terminal = new XTerm({
      cursorBlink: !window.matchMedia?.('(prefers-reduced-motion: reduce)').matches,
      convertEol: true,
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
      fontSize: 13,
      scrollback: 3000,
      lineHeight: 1.25,
      theme: terminalThemeFromTokens(terminalRef.current),
    })
    const fitAddon = new FitAddon()
    terminal.loadAddon(fitAddon)
    terminal.open(terminalRef.current)
    terminal.writeln(t('deploymentsPage.webConsoleConnecting'))
    terminal.focus()

    let socket: WebSocket | undefined
    let failed = false

    const sendResize = () => {
      if (!socket || socket.readyState !== WebSocket.OPEN)
        return
      socket.send(JSON.stringify({ type: 'resize', cols: terminal.cols, rows: terminal.rows }))
    }

    const fitAndResize = () => {
      fitAddon.fit()
      sendResize()
    }

    const dataSubscription = terminal.onData((data) => {
      if (socket?.readyState === WebSocket.OPEN)
        socket.send(encodeRuntimeTerminalInput(data))
    })
    const resizeObserver = new ResizeObserver(fitAndResize)
    resizeObserver.observe(terminalRef.current)

    const handleOpen = () => {
      fitAndResize()
      onConnectionChange?.('connected')
      terminal.writeln(t('deploymentsPage.webConsoleConnected'))
    }
    const handleMessage = (event: MessageEvent) => {
      if (typeof event.data === 'string') {
        const control = parseRuntimeTerminalControl(event.data)
        terminal.options.disableStdin = true
        if (!control)
          socket?.close(1002, 'invalid terminal control message')
        return
      }
      terminal.write(new Uint8Array(event.data))
    }
    const handleClose = () => {
      if (!failed)
        onConnectionChange?.('disconnected')
      terminal.writeln('')
      terminal.writeln(t('deploymentsPage.webConsoleDisconnected'))
    }
    const handleError = () => {
      failed = true
      onConnectionChange?.('failed')
      terminal.writeln('')
      terminal.writeln(t('deploymentsPage.webConsoleConnectionFailed'))
    }

    socket = createTracedWebSocket(terminalSocketUrl, RUNTIME_TERMINAL_WEBSOCKET_SUBPROTOCOL, 'runtime.terminal.websocket')
    socket.binaryType = 'arraybuffer'
    socket.addEventListener('open', handleOpen)
    socket.addEventListener('message', handleMessage)
    socket.addEventListener('close', handleClose)
    socket.addEventListener('error', handleError)

    const fitTimer = window.setTimeout(fitAndResize, 50)
    window.addEventListener('resize', fitAndResize)

    return () => {
      window.clearTimeout(fitTimer)
      window.removeEventListener('resize', fitAndResize)
      socket?.removeEventListener('open', handleOpen)
      socket?.removeEventListener('message', handleMessage)
      socket?.removeEventListener('close', handleClose)
      socket?.removeEventListener('error', handleError)
      resizeObserver.disconnect()
      dataSubscription.dispose()
      socket?.close()
      terminal.dispose()
    }
  }, [container, onConnectionChange, pod, projectId, ready, release, socketUrl, t])

  return (
    <div className={cn('bg-terminal p-3', fullscreen && 'flex h-full min-h-0')}>
      <div ref={terminalRef} className={fullscreen ? 'min-h-0 flex-1 overflow-hidden' : 'h-[28rem] overflow-hidden'} />
    </div>
  )
}

/** xterm 只接受具体颜色值，从终端 token 读取，保持与日志查看器一致。 */
function terminalThemeFromTokens(element: HTMLElement): ITheme {
  const styles = getComputedStyle(element)
  const token = (name: string) => styles.getPropertyValue(`--terminal-${name}`).trim() || undefined
  const background = token('background')
  const foreground = token('foreground')
  const muted = token('muted')
  return {
    background,
    foreground,
    cursor: token('cursor'),
    cursorAccent: background,
    selectionBackground: token('selection'),
    black: background,
    brightBlack: muted,
    red: token('error'),
    brightRed: token('error'),
    green: token('success'),
    brightGreen: token('success'),
    yellow: token('warning'),
    brightYellow: token('warning'),
    blue: token('info'),
    brightBlue: token('info'),
    magenta: token('magenta'),
    brightMagenta: token('magenta'),
    cyan: token('cyan'),
    brightCyan: token('cyan'),
    white: muted,
    brightWhite: foreground,
  }
}
