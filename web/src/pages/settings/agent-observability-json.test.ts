import { describe, expect, it } from 'vitest'
import { formatObservabilityJSON, jsonSyntaxTokens } from './agent-observability-json-utils'

describe('observability JSON presentation', () => {
  it('formats nested tool payloads and classifies syntax tokens', () => {
    const value = { projectId: 'proj_1', count: 2, approved: true, result: null }
    expect(formatObservabilityJSON(value)).toContain('\n  "projectId": "proj_1"')
    expect(jsonSyntaxTokens(value).filter(token => token.kind !== 'plain').map(({ kind, value }) => ({ kind, value }))).toEqual([
      { kind: 'key', value: '"projectId"' },
      { kind: 'string', value: '"proj_1"' },
      { kind: 'key', value: '"count"' },
      { kind: 'number', value: '2' },
      { kind: 'key', value: '"approved"' },
      { kind: 'boolean', value: 'true' },
      { kind: 'key', value: '"result"' },
      { kind: 'null', value: 'null' },
    ])
  })
})
