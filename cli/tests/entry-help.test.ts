import { describe, expect, it } from 'vitest'
import { commandHelpText, rootHelpText } from '../src/commands/human-help.js'
import { createLunaCli, startupOptionValue } from '../src/entry.js'
import { createCliI18n, normalizeLocale } from '../src/i18n/index.js'

describe('cli startup and human help', () => {
  it('reads startup options before Commander renders help', () => {
    expect(startupOptionValue(['node', 'luna', '--lang', 'zh-CN'], 'lang')).toBe('zh-CN')
    expect(startupOptionValue(['node', 'luna', '--lang=zh-CN'], 'lang')).toBe('zh-CN')
    expect(startupOptionValue(['node', 'luna', 'version', 'show', 'lang=zh-CN'], 'lang')).toBe('zh-CN')
  })

  it('renders localized, actionable root and command help', async () => {
    const i18n = await createCliI18n({ env: { LUNA_LANG: 'zh-CN' } })
    const cli = createLunaCli({
      ports: {
        translate(key, fallback, locale) {
          return i18n.getFixedT(normalizeLocale(locale) ?? i18n.language)(key, {
            defaultValue: fallback,
          })
        },
      },
    })

    const rootHelp = [
      cli.program.helpInformation(),
      rootHelpText(cli.registry, cli.ports),
    ].join('\n')
    expect(rootHelp).toContain('快速开始：')
    expect(rootHelp).toContain('业务参数统一使用 key=value')
    expect(rootHelp).toContain('当前共有 19 个分类、131 条命令')

    const helpCategory = cli.program.commands.find(command => command.name() === 'help')
    const command = helpCategory?.commands.find(item => item.name() === 'command')
    const metadata = cli.registry.get('help.command')?.metadata
    const commandHelp = [
      command?.helpInformation() ?? '',
      metadata ? commandHelpText(metadata, cli.ports) : '',
    ].join('\n')
    expect(commandHelp).toContain('业务参数：')
    expect(commandHelp).toContain('path=<value>  [必填, string')
    expect(commandHelp).toContain('luna help command path=project.get-projects output=json')
  })
})
