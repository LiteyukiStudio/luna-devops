import { describe, expect, it } from 'vitest'
import { parseLogLines, stripAnsi } from './log-lines'

describe('log lines', () => {
  it('marks error and warning lines without flagging plain prose', () => {
    const lines = parseLogLines([
      '#7 ERROR: failed to solve: process did not complete',
      'npm ERR! missing script: build',
      '#6 9.802 ERR_PNPM_FETCH_404  GET https://registry.npmjs.org/x: Not Found',
      'level=warn msg="slow request"',
      'WARNING: image platform does not match',
      'compiled with 0 errors and no warnings',
      '',
    ].join('\n'))

    expect(lines.map(line => line.severity)).toEqual(['error', 'error', 'error', 'warning', 'warning', undefined])
  })

  it('maps ANSI colors to semantic tones and strips other control sequences', () => {
    const [line] = parseLogLines('\u001B[1;31mfail\u001B[0m done\u001B[2K')

    expect(line.segments).toEqual([
      { bold: true, text: 'fail', tone: 'error' },
      { bold: false, text: ' done', tone: undefined },
    ])
    expect(stripAnsi('\u001B[32mok\u001B[39m\r\nprogress 10%\rprogress 100%')).toBe('ok\nprogress 100%')
  })
})
