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
  const ports: RuntimePorts = {
    config,
    input: options.ports?.input ?? new DefaultInputPort(),
    output,
    api: options.ports?.api ?? new LunaApiAdapter({ config, env }),
    env,
    isTTY: options.ports?.isTTY ?? Boolean(process.stdout.isTTY),
    version,
    distribution: options.distribution ?? options.ports?.distribution ?? runtimeDistribution(),
    translate,
  }
  const registry = createRegistryFromContract(apiContract)
  registerLocalCommands(registry)
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
  const i18n = await createCliI18n({ env: process.env })
  const cli = createLunaCli({
    ports: {
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
