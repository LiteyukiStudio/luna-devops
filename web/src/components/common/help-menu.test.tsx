import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it } from 'vitest'
import i18next from '@/i18n'
import { HelpMenu } from './help-menu'

describe('help menu', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('zh-CN')
  })

  it('opens documentation and GitHub repository external links from the topbar help button', async () => {
    const interaction = userEvent.setup()
    render(<HelpMenu />)

    await interaction.click(screen.getByRole('button', { name: '帮助与资源' }))

    const docsItem = screen.getByRole('menuitem', { name: '文档' })
    const githubItem = screen.getByRole('menuitem', { name: 'GitHub 仓库' })

    expect(docsItem).toHaveAttribute('href', 'https://luna-devops.liteyuki.org')
    expect(docsItem).toHaveAttribute('target', '_blank')
    expect(docsItem).toHaveAttribute('rel', 'noreferrer')

    expect(githubItem).toHaveAttribute('href', 'https://github.com/LiteyukiStudio/luna-devops')
    expect(githubItem).toHaveAttribute('target', '_blank')
    expect(githubItem).toHaveAttribute('rel', 'noreferrer')
  })
})
