import type { CurrentUser } from '@/api'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { SidebarUserPanel } from './sidebar-user-panel'

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

describe('sidebar user panel', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('zh-CN')
  })

  it('groups help links and logout in the account menu', async () => {
    const interaction = userEvent.setup()
    render(<SidebarUserPanel user={user} onLogout={() => undefined} />)

    await interaction.click(screen.getByRole('button', { name: '账号菜单' }))

    expect(screen.getByRole('menuitem', { name: 'GitHub 仓库' })).toHaveAttribute('href', 'https://github.com/LiteyukiStudio/luna-devops')
    expect(screen.getByRole('menuitem', { name: '文档' })).toHaveAttribute('href', 'https://luna-devops.liteyuki.org')
    expect(screen.getByRole('menuitem', { name: '退出' })).toBeInTheDocument()
  })

  it('requires confirmation before logging out', async () => {
    const interaction = userEvent.setup()
    const onLogout = vi.fn()
    render(<SidebarUserPanel user={user} onLogout={onLogout} />)

    await interaction.click(screen.getByRole('button', { name: '账号菜单' }))
    await interaction.click(screen.getByRole('menuitem', { name: '退出' }))

    expect(onLogout).not.toHaveBeenCalled()
    expect(screen.getByRole('heading', { name: '确认退出登录？' })).toBeInTheDocument()

    await interaction.click(screen.getByRole('button', { name: '退出' }))
    expect(onLogout).toHaveBeenCalledOnce()
  })
})
