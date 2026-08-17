import { describe, expect, it } from 'vitest'
import { formatKeyValueText, parseKeyValueText } from './key-value-text'

describe('key-value text', () => {
  it('parses first separator and preserves internal value equals', () => {
    expect(parseKeyValueText('URL=https://example.test?a=b\nNAME=demo')).toEqual({ URL: 'https://example.test?a=b', NAME: 'demo' })
  })

  it('formats keys in stable order', () => {
    expect(formatKeyValueText({ Z: 'last', A: 'first' })).toBe('A=first\nZ=last')
  })

  it('rejects empty and duplicate keys', () => {
    expect(() => parseKeyValueText('=value')).toThrow()
    expect(() => parseKeyValueText('TOKEN=one\nTOKEN=two')).toThrow()
  })
})
