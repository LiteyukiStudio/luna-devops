import { describe, expect, it } from 'vitest'
import { CliCommandError, CommandRegistry } from '../../src/commands/index.js'

describe('commandRegistry', () => {
  it('enforces the fixed two-level canonical path', () => {
    const registry = new CommandRegistry()
    const command = registry.register({
      category: 'project',
      tool: 'list',
      source: 'local',
      aliases: ['ls'],
    }, async () => ({ data: [] }))

    expect(command.metadata.canonicalPath).toBe('project.list')
    expect(registry.get('project.ls', true)).toBe(command)
    expect(() => registry.get('project.ls')).not.toThrow()
    expect(registry.get('project.ls')).toBeUndefined()
  })

  it('rejects duplicate canonical commands', () => {
    const registry = new CommandRegistry()
    const metadata = { category: 'project', tool: 'list', source: 'local' } as const
    registry.register(metadata, async () => ({ data: [] }))

    expect(() => registry.register(metadata, async () => ({ data: [] })))
      .toThrowError(CliCommandError)
  })

  it('prefers canonical commands over generated aliases', () => {
    const registry = new CommandRegistry()
    const generated = registry.register({
      category: 'auth',
      tool: 'post-auth-login',
      source: 'openapi',
      operationId: 'login',
      aliases: ['login'],
    }, async () => ({ data: 'generated' }))
    const local = registry.register({
      category: 'auth',
      tool: 'login',
      source: 'local',
    }, async () => ({ data: 'local' }))

    expect(registry.get('auth.login', true)).toBe(local)
    expect(registry.get('auth.post-auth-login')).toBe(generated)
  })
})
