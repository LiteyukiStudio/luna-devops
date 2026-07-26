import type { CliProgramOptions, RuntimePorts } from './commands/index.js'
import process from 'node:process'
import { pathToFileURL } from 'node:url'
import * as apiContract from '@luna-devops/api-contract'
import {
  CommandOutput,
  createCliProgram,
  createRegistryFromContract,
  DefaultInputPort,
  LunaApiAdapter,
  registerLocalCommands,
  runCli,
} from './commands/index.js'
import { FileConfigStore } from './config/index.js'
import { createCliI18n, normalizeLocale } from './i18n/index.js'
import { CLI_VERSION } from './version.js'

export interface LunaCliOptions {
  readonly ports?: Partial<RuntimePorts>
  readonly version?: string
  readonly distribution?: RuntimePorts['distribution']
}

export function createLunaCli(options: LunaCliOptions = {}) {
  const version = options.version ?? CLI_VERSION
  const env = options.ports?.env ?? process.env
  const config = options.ports?.config ?? new FileConfigStore()
  const translate = options.ports?.translate
  const output = options.ports?.output ?? new CommandOutput({ version, translate })
  const registry = createRegistryFromContract(apiContract)
  registerLocalCommands(registry)
  const ports: RuntimePorts = {
    config,
    input: options.ports?.input ?? new DefaultInputPort(),
    output,
    api: options.ports?.api ?? new LunaApiAdapter({
      config,
      env,
      compatibility: {
        cliVersion: version,
        openapiDigest: registry.catalogMetadata.openapiDigest,
      },
    }),
    env,
    isTTY: options.ports?.isTTY ?? Boolean(process.stdout.isTTY),
    version,
    distribution: options.distribution ?? options.ports?.distribution ?? runtimeDistribution(),
    translate,
  }
  const programOptions: CliProgramOptions = {
    registry,
    ports,
    name: 'luna',
    description: translate?.(
      'cli.description',
      'Luna DevOps command-line client for people and agents',
    ) ?? 'Luna DevOps command-line client for people and agents',
  }
  return {
    program: createCliProgram(programOptions),
    registry,
    ports,
  }
}

export async function main(argv: readonly string[] = process.argv): Promise<number> {
  const env = process.env
  const config = new FileConfigStore()
  const configuredLanguage = await startupConfiguredLanguage(config)
  const i18n = await createCliI18n({
    explicit: startupOptionValue(argv, 'lang'),
    configured: configuredLanguage,
    env,
  })
  const cli = createLunaCli({
    ports: {
      config,
      env,
      translate(key, fallback, locale) {
        return i18n.getFixedT(normalizeLocale(locale) ?? i18n.language)(key, {
          defaultValue: fallback,
        })
      },
    },
  })
  const result = await runCli(cli.program, argv, cli.ports.output)
  process.exitCode = result.exitCode
  return result.exitCode
}

export function startupOptionValue(
  argv: readonly string[],
  name: string,
): string | undefined {
  const flag = `--${name}`
  const canonical = `${name}=`
  for (let index = 0; index < argv.length; index += 1) {
    const token = argv[index]
    if (token === flag)
      return argv[index + 1]
    if (token?.startsWith(`${flag}=`))
      return token.slice(flag.length + 1)
    if (token?.startsWith(canonical))
      return token.slice(canonical.length)
  }
  return undefined
}

async function startupConfiguredLanguage(
  configStore: FileConfigStore,
): Promise<string | undefined> {
  try {
    const config = await configStore.read()
    return config.language || undefined
  }
  catch {
    // Help and recovery commands must remain usable when local config is malformed.
    return undefined
  }
}

function runtimeDistribution(): RuntimePorts['distribution'] {
  if (typeof process.versions.bun === 'string')
    return 'binary'
  return process.env.npm_package_name ? 'npm' : 'source'
}

if (isDirectExecution()) {
  void main()
}

function isDirectExecution(): boolean {
  const executable = process.argv[1]
  return Boolean(executable && import.meta.url === pathToFileURL(executable).href)
}
