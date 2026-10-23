export interface DotEnvEntry {
  key: string
  value: string
}

export interface DotEnvParseResult {
  entries: DotEnvEntry[]
  /** 无法解析的行号（从 1 开始），不会被导入。 */
  invalidLines: number[]
}

const dotEnvKeyPattern = /^[A-Z_]\w{0,127}$/i

/**
 * 解析常见 .env 文本：支持 `export KEY=VALUE`、整行 `#` 注释与成对引号。
 * 同名键以最后一次出现为准；值中的 `#` 原样保留，避免误删 URL 或密码片段。
 */
export function parseDotEnv(text: string): DotEnvParseResult {
  const values = new Map<string, string>()
  const invalidLines: number[] = []
  text.split(/\r?\n/).forEach((rawLine, index) => {
    const line = rawLine.trim()
    if (!line || line.startsWith('#'))
      return
    const assignment = line.startsWith('export ') ? line.slice('export '.length).trimStart() : line
    const separator = assignment.indexOf('=')
    const key = separator > 0 ? assignment.slice(0, separator).trim() : ''
    if (!dotEnvKeyPattern.test(key)) {
      invalidLines.push(index + 1)
      return
    }
    values.set(key, unquote(assignment.slice(separator + 1).trim()))
  })
  return {
    entries: [...values].map(([key, value]) => ({ key, value })),
    invalidLines,
  }
}

function unquote(value: string) {
  if (value.length >= 2) {
    const quote = value[0]
    if ((quote === '"' || quote === '\'') && value.endsWith(quote)) {
      const inner = value.slice(1, -1)
      return quote === '"' ? inner.replaceAll('\\n', '\n').replaceAll('\\"', '"') : inner
    }
  }
  return value
}
