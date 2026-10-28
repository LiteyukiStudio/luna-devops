import { describe, expect, it } from 'vitest'
import { parseDotEnv } from './dotenv'

describe('parseDotEnv', () => {
  it('parses exports, comments, quotes and keeps the last duplicate', () => {
    const result = parseDotEnv([
      '# database',
      'export DATABASE_URL=postgres://db/app#primary',
      'LOG_LEVEL="info"',
      'GREETING=\'hello world\'',
      'MULTILINE="a\\nb"',
      'LOG_LEVEL=debug',
      '',
    ].join('\n'))

    expect(result.invalidLines).toEqual([])
    expect(result.entries).toEqual([
      { key: 'DATABASE_URL', value: 'postgres://db/app#primary' },
      { key: 'LOG_LEVEL', value: 'debug' },
      { key: 'GREETING', value: 'hello world' },
      { key: 'MULTILINE', value: 'a\nb' },
    ])
  })

  it('reports lines that cannot become environment variables', () => {
    expect(parseDotEnv('NO_SEPARATOR\n1BAD=x\n=empty\nOK=1').invalidLines).toEqual([1, 2, 3])
  })
})
