import type { CurrentUser } from '@/api'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { AccountMenu } from './account-menu'

const user: CurrentUser = {
  avatarUrl: '',
  brandColorPreset: '',
  email: 'snowykami@example.com',
  id: 'usr_test',
  interfaceStyle: '',
  language: 'zh-CN',
  name: 'snowykami',
  passwordSet: true,
  permissions: [],
  role: 'user',
}

function renderAccountMenu(onLogout: () => void = () => undefined) {
  return render(
    <MemoryRouter>
      <AccountMenu user={user} onLogout={onLogout} />
    </MemoryRouter>,
  )
}

describe('account menu', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('zh-CN')
  })

  it('shows identity, account and help destinations from the avatar', async () => {
    const interaction = userEvent.setup()
    renderAccountMenu()

    await interaction.click(screen.getByRole('button', { name: '账号菜单' }))

    expect(screen.getByText('snowykami')).toBeVisible()
    expect(screen.getByText('snowykami@example.com')).toBeVisible()
    expect(screen.getByRole('menuitem', { name: '账号' })).toHaveAttribute('href', '/settings/account')
    expect(screen.getByRole('menuitem', { name: 'GitHub 仓库' })).toHaveAttribute('href', 'https://github.com/LiteyukiStudio/luna-devops')
    expect(screen.getByRole('menuitem', { name: '文档' })).toHaveAttribute('href', 'https://luna-devops.liteyuki.org')
    expect(screen.getByRole('menuitem', { name: '退出' })).toBeInTheDocument()
  })

  it('requires confirmation before logging out', async () => {
    const interaction = userEvent.setup()
    const onLogout = vi.fn()
    renderAccountMenu(onLogout)

    await interaction.click(screen.getByRole('button', { name: '账号菜单' }))
    await interaction.click(screen.getByRole('menuitem', { name: '退出' }))

    expect(onLogout).not.toHaveBeenCalled()
    expect(screen.getByRole('heading', { name: '确认退出登录？' })).toBeInTheDocument()

    await interaction.click(screen.getByRole('button', { name: '退出' }))
    expect(onLogout).toHaveBeenCalledOnce()
  })
})
