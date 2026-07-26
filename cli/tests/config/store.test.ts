import { lstat, mkdtemp, readFile, symlink } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'

import { afterEach, describe, expect, it } from 'vitest'

import {
  emptyConfigDocument,
  FileConfigStore,
  resolveConfigPath,
} from '../../src/config/index.js'

const temporaryDirectories: string[] = []

afterEach(async () => {
  const { rm } = await import('node:fs/promises')
  await Promise.all(
    temporaryDirectories.splice(0).map(directory =>
      rm(directory, { force: true, recursive: true })),
  )
})

describe('resolveConfigPath', () => {
  it('uses explicit paths before environment and defaults', () => {
    expect(
      resolveConfigPath({
        configPath: './explicit.json',
        env: { LUNA_CONFIG: './environment.json' },
        homeDir: '/home/luna',
      }),
    ).toBe(path.resolve('./explicit.json'))
  })

  it('supports a test home override', () => {
    expect(resolveConfigPath({ homeDir: '/tmp/luna-home', env: {} })).toBe(
      '/tmp/luna-home/.luna/auth.json',
    )
  })

  it('uses LUNA_CONFIG when no explicit path is provided', () => {
    expect(
      resolveConfigPath({
        env: { LUNA_CONFIG: '~/isolated/auth.json' },
        homeDir: '/tmp/luna-home',
      }),
    ).toBe('/tmp/luna-home/isolated/auth.json')
  })
})

describe('fileConfigStore', () => {
  it('writes atomically with private directory and file permissions', async () => {
    const directory = await temporaryDirectory()
    const configPath = path.join(directory, '.luna', 'auth.json')
    const store = new FileConfigStore({ configPath })
    const config = emptyConfigDocument()
    config.server = 'https://devops.example.com'
    config.credential = {
      type: 'access_token',
      token: 'secret',
      scopes: [],
    }

    await store.write(config)

    expect(await store.read()).toEqual(config)
    expect((await lstat(path.dirname(configPath))).mode & 0o777).toBe(0o700)
    expect((await lstat(configPath)).mode & 0o777).toBe(0o600)
    expect(await readFile(configPath, 'utf8')).toContain(
      '"server": "https://devops.example.com"',
    )
  })

  it('serializes concurrent read-modify-write operations with a lock', async () => {
    const directory = await temporaryDirectory()
    const store = new FileConfigStore({
      configPath: path.join(directory, '.luna', 'auth.json'),
    })

    await Promise.all(
      Array.from({ length: 8 }, (_, index) =>
        store.update((config) => {
          config.language = `language-${index}`
        })),
    )

    expect((await store.read()).language).toMatch(/^language-\d$/u)
  })

  it('refuses to follow a symbolic-link config file', async () => {
    const directory = await temporaryDirectory()
    const target = path.join(directory, 'target.json')
    const configPath = path.join(directory, 'auth.json')
    await import('node:fs/promises').then(fs =>
      fs.writeFile(target, JSON.stringify(emptyConfigDocument()), { mode: 0o600 }))
    await symlink(target, configPath)
    const store = new FileConfigStore({ configPath })

    await expect(store.read()).rejects.toMatchObject({
      code: 'config_path_unsafe',
    })
  })

  it('refuses persistent credential storage on Windows without a DACL backend', async () => {
    const directory = await temporaryDirectory()
    const store = new FileConfigStore({
      configPath: path.join(directory, '.luna', 'auth.json'),
      platform: 'win32',
    })

    await expect(store.write(emptyConfigDocument())).rejects.toMatchObject({
      code: 'secure_storage_unavailable',
      status: 501,
    })
  })
})

async function temporaryDirectory(): Promise<string> {
  const directory = await mkdtemp(path.join(os.tmpdir(), 'luna-cli-config-'))
  temporaryDirectories.push(directory)
  return directory
}
