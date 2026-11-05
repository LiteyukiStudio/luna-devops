export type LogTone = 'error' | 'info' | 'muted' | 'success' | 'warning'

export interface LogSegment {
  bold: boolean
  text: string
  tone?: LogTone
}

export interface LogLine {
  segments: LogSegment[]
  severity?: 'error' | 'warning'
}

const ESCAPE = '\u001B'

// 大写关键字区分大小写，小写形式只在 `error:` / `level=error` 等结构化写法里识别，避免 “0 errors” 误报。
const errorPattern = /\b(?:ERROR|ERRO|FATAL|FAILED|FAILURE|PANIC|CRITICAL)\b|\bERR!|\bERR_[A-Z0-9_]+\b|\b(?:error|fatal|panic|Error|Fatal|Panic)(?::|\])|\blevel=(?:error|fatal|panic)\b|"level":\s*"(?:error|fatal|panic)"/
const warningPattern = /\b(?:WARN|WARNING)\b|\b(?:warn|warning|Warn|Warning)(?::|\])|\blevel=warn(?:ing)?\b|"level":\s*"warn(?:ing)?"/

/** 把原始日志拆成行，解析 ANSI SGR 颜色并按关键字标记错误/警告行。 */
export function parseLogLines(content: string): LogLine[] {
  const lines = content.replace(/\r\n/g, '\n').split('\n')
  if (lines.length > 1 && lines.at(-1) === '')
    lines.pop()
  return lines.map(parseLogLine)
}

/** 去掉 ANSI 控制序列，供复制与下载使用。 */
export function stripAnsi(content: string) {
  return parseLogLines(content).map(line => line.segments.map(segment => segment.text).join('')).join('\n')
}

function parseLogLine(raw: string): LogLine {
  const segments = parseAnsiSegments(lastCarriageReturnFrame(raw))
  const text = segments.map(segment => segment.text).join('')
  const severity = errorPattern.test(text) ? 'error' : warningPattern.test(text) ? 'warning' : undefined
  return { segments, severity }
}

// 进度条类输出用 \r 覆盖同一行，只保留最后一帧。
function lastCarriageReturnFrame(raw: string) {
  if (!raw.includes('\r'))
    return raw
  const frames = raw.split('\r').filter(Boolean)
  return frames.at(-1) ?? ''
}

function parseAnsiSegments(raw: string): LogSegment[] {
  const segments: LogSegment[] = []
  let tone: LogTone | undefined
  let bold = false
  let text = ''
  let index = 0

  const flush = () => {
    if (!text)
      return
    segments.push({ bold, text, tone })
    text = ''
  }

  while (index < raw.length) {
    const char = raw[index]
    if (char !== ESCAPE) {
      text += char
      index += 1
      continue
    }
    if (raw[index + 1] !== '[') {
      index += 2
      continue
    }
    let end = index + 2
    while (end < raw.length && !isCsiFinalByte(raw.charCodeAt(end)))
      end += 1
    if (raw[end] === 'm') {
      flush()
      ;({ bold, tone } = applySgr(raw.slice(index + 2, end), { bold, tone }))
    }
    index = end + 1
  }
  flush()
  return segments.length > 0 ? segments : [{ bold: false, text: '' }]
}

function isCsiFinalByte(code: number) {
  return code >= 0x40 && code <= 0x7E
}

function applySgr(params: string, state: { bold: boolean, tone?: LogTone }) {
  const codes = params === '' ? [0] : params.split(';').map(Number)
  let { bold, tone } = state
  for (let index = 0; index < codes.length; index += 1) {
    const code = codes[index]
    if (code === 0) {
      bold = false
      tone = undefined
    }
    else if (code === 1) {
      bold = true
    }
    else if (code === 22) {
      bold = false
    }
    else if (code === 39) {
      tone = undefined
    }
    else if (code === 38 || code === 48) {
      // 256 色与真彩色参数不映射到语义色，跳过其参数。
      index += codes[index + 1] === 5 ? 2 : codes[index + 1] === 2 ? 4 : 0
    }
    else if ((code >= 30 && code <= 37) || (code >= 90 && code <= 97)) {
      tone = ansiTone(code % 10)
    }
  }
  return { bold, tone }
}

function ansiTone(color: number): LogTone | undefined {
  switch (color) {
    case 0:
      return 'muted'
    case 1:
      return 'error'
    case 2:
      return 'success'
    case 3:
      return 'warning'
    case 4:
    case 5:
    case 6:
      return 'info'
    default:
      return undefined
  }
}
