import type { Release } from '@/api'
import { FitAddon } from '@xterm/addon-fit'
import { Terminal as XTerm } from '@xterm/xterm'
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { releaseRuntimeTerminalUrl } from '@/api'
import '@xterm/xterm/css/xterm.css'

export function ApplicationRuntimeTerminalPanel({
  container,
  fullscreen = false,
  projectId,
  release,
}: {
  container: string
  fullscreen?: boolean
  projectId: string
  release: Release | null
}) {
  const { t } = useTranslation()
  const terminalRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!release || !projectId || !terminalRef.current)
      return

    const terminal = new XTerm({
      cursorBlink: true,
      convertEol: true,
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
      fontSize: 13,
      scrollback: 3000,
      theme: {
        background: '#020617',
        black: '#020617',
        blue: '#60a5fa',
        brightBlack: '#475569',
        brightBlue: '#93c5fd',
        brightCyan: '#67e8f9',
        brightGreen: '#86efac',
        brightMagenta: '#f0abfc',
        brightRed: '#fca5a5',
        brightWhite: '#f8fafc',
        brightYellow: '#fde68a',
        cursor: '#34d399',
        cyan: '#22d3ee',
        foreground: '#e2e8f0',
        green: '#22c55e',
        magenta: '#d946ef',
        red: '#ef4444',
        white: '#cbd5e1',
        yellow: '#facc15',
      },
    })
    const fitAddon = new FitAddon()
    terminal.loadAddon(fitAddon)
    terminal.open(terminalRef.current)
    terminal.writeln(t('deploymentsPage.webConsoleConnecting'))
    terminal.focus()

    const socket = new WebSocket(releaseRuntimeTerminalUrl(projectId, release.id, container))
    socket.binaryType = 'arraybuffer'

    const sendResize = () => {
      if (socket.readyState !== WebSocket.OPEN)
        return
      socket.send(JSON.stringify({ type: 'resize', cols: terminal.cols, rows: terminal.rows }))
    }

    const fitAndResize = () => {
      fitAddon.fit()
      sendResize()
    }

    const dataSubscription = terminal.onData((data) => {
      if (socket.readyState === WebSocket.OPEN)
        socket.send(data)
    })
    const resizeObserver = new ResizeObserver(fitAndResize)
    resizeObserver.observe(terminalRef.current)

    const handleOpen = () => {
      fitAndResize()
      terminal.writeln(t('deploymentsPage.webConsoleConnected'))
    }
    const handleMessage = (event: MessageEvent) => {
      if (typeof event.data === 'string') {
        terminal.write(event.data)
        return
      }
      terminal.write(new Uint8Array(event.data))
    }
    const handleClose = () => {
      terminal.writeln('')
      terminal.writeln(t('deploymentsPage.webConsoleDisconnected'))
    }
    const handleError = () => {
      terminal.writeln('')
      terminal.writeln(t('deploymentsPage.webConsoleConnectionFailed'))
    }

    socket.addEventListener('open', handleOpen)
    socket.addEventListener('message', handleMessage)
    socket.addEventListener('close', handleClose)
    socket.addEventListener('error', handleError)

    const fitTimer = window.setTimeout(fitAndResize, 50)
    window.addEventListener('resize', fitAndResize)

    return () => {
      window.clearTimeout(fitTimer)
      window.removeEventListener('resize', fitAndResize)
      socket.removeEventListener('open', handleOpen)
      socket.removeEventListener('message', handleMessage)
      socket.removeEventListener('close', handleClose)
      socket.removeEventListener('error', handleError)
      resizeObserver.disconnect()
      dataSubscription.dispose()
      socket.close()
      terminal.dispose()
    }
  }, [container, projectId, release, t])

  return (
    <div className={fullscreen ? 'flex h-full min-h-0 p-3 pt-2' : 'p-3'}>
      <div ref={terminalRef} className={fullscreen ? 'min-h-0 flex-1 overflow-hidden rounded border border-zinc-800 bg-slate-950 p-2' : 'h-[28rem] overflow-hidden rounded border border-zinc-800 bg-slate-950 p-2'} />
    </div>
  )
}
