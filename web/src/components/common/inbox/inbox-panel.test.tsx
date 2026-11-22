import type { InboxMessage } from '@/api'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { InboxPanel } from './inbox-panel'

const mocks = vi.hoisted(() => ({
  getInboxUnreadCount: vi.fn(),
  listInboxMessages: vi.fn(),
  markInboxMessageRead: vi.fn(),
  markAllInboxMessagesRead: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      ...mocks,
    },
  }
})

function LocationProbe() {
  const location = useLocation()
  return <div data-testid="current-route">{`${location.pathname}${location.search}${location.hash}`}</div>
}

function renderPanel(onClose = vi.fn()) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <MemoryRouter initialEntries={['/projects']}>
      <QueryClientProvider client={queryClient}>
        <div className="flex max-h-[36rem] w-[26rem] flex-col overflow-hidden">
          <InboxPanel onClose={onClose} />
        </div>
        <LocationProbe />
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

const buildFailedMessage: InboxMessage = {
  id: 'imsg_build_fail_1',
  type: 'build.failed',
  category: 'delivery',
  priority: 'high',
  actorId: '',
  projectId: 'prj_1',
  resourceType: 'build_run',
  resourceId: 'br_101',
  titleKey: 'inbox.delivery.buildFailed.title',
  contentKey: 'inbox.delivery.buildFailed.body',
  params: {
    projectName: 'Snowykami 项目空间',
    applicationName: 'wordle-helper',
    targetName: 'prod',
    gitRef: 'main',
    reason: 'failed to solve: nginxx:1.27: not found',
    aiRootCauseSummary: 'Dockerfile 第 1 行基础镜像名拼写错误：写成 `nginxx:1.27`，应改为 `nginx:1.27`',
  },
  actionRequestId: '',
  deepLink: '/projects/prj_1/apps/app_1?tab=builds#buildRunId=br_101',
  groupKey: '',
  createdAt: '2026-11-22T14:30:00Z',
  updatedAt: '2026-11-22T14:30:00Z',
}

describe('inbox panel popover UI/UX', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    await i18next.changeLanguage('zh-CN')
    mocks.getInboxUnreadCount.mockResolvedValue({ unreadCount: 1 })
    mocks.listInboxMessages.mockResolvedValue({
      items: [buildFailedMessage],
      page: 1,
      pageSize: 10,
      sortBy: 'createdAt',
      sortOrder: 'desc',
      total: 1,
      totalPages: 1,
    })
    mocks.markInboxMessageRead.mockResolvedValue({})
    mocks.markAllInboxMessagesRead.mockResolvedValue({})
  })

  it('renders filter tabs, unread badge, formatted AI summary without raw backticks, and navigates to deepLink', async () => {
    const onClose = vi.fn()
    const user = userEvent.setup()
    renderPanel(onClose)

    expect(await screen.findByText('构建失败')).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '全部' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '未读' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '待处理' })).toBeInTheDocument()
    expect(screen.getByText('Dockerfile 第 1 行基础镜像名拼写错误：写成 nginxx:1.27，应改为 nginx:1.27')).toBeInTheDocument()

    const markAllButton = screen.getByRole('button', { name: '全部标记已读' })
    expect(markAllButton).not.toBeDisabled()

    await user.click(screen.getByRole('button', { name: /构建失败/ }))
    expect(onClose).toHaveBeenCalledTimes(1)
    expect(mocks.markInboxMessageRead).toHaveBeenCalledWith('imsg_build_fail_1')
    await waitFor(() => {
      expect(screen.getByTestId('current-route')).toHaveTextContent('/projects/prj_1/apps/app_1?tab=builds#buildRunId=br_101')
    })
  })

  it('disables mark-all-read when unread count is zero and renders error retry state on load failure', async () => {
    mocks.getInboxUnreadCount.mockResolvedValue({ unreadCount: 0 })
    mocks.listInboxMessages.mockRejectedValueOnce(new Error('network error'))

    renderPanel()

    expect(await screen.findByText('消息加载失败')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '全部标记已读' })).toBeDisabled()
  })
})
