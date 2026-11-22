import type { ToolCallBlock } from './tool-call'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import i18next from '@/i18n'
import { AIToolCallCard } from './tool-call'

vi.mock('@/app/session-context', () => ({
  useSession: () => ({
    user: { email: 'admin@example.test' },
  }),
}))

function toolBlock(status: ToolCallBlock['status']): ToolCallBlock {
  return {
    id: 'tool-item',
    turnId: 'turn-1',
    runId: 'run-1',
    index: 0,
    type: 'tool_call',
    toolCallId: 'tool-call-1',
    operationId: 'listApplications',
    visibility: 'normal',
    status,
    arguments: {},
    uiActions: [],
  }
}

describe('ai assistant tool status icon', () => {
  it('shows a running status icon only for a genuinely running tool call', async () => {
    await i18next.changeLanguage('zh-CN')
    const { container } = render(
      <AIToolCallCard
        block={toolBlock('running')}
        onAction={vi.fn(async () => true)}
        onApproval={vi.fn(async () => {})}
      />,
    )

    expect(container.querySelector('[data-ai-tool-status-icon="running"]')).toBeInTheDocument()
  })

  it('replaces the running icon after a tool call fails', () => {
    const { container } = render(
      <AIToolCallCard
        block={toolBlock('failed')}
        onAction={vi.fn(async () => true)}
        onApproval={vi.fn(async () => {})}
      />,
    )

    expect(container.querySelector('[data-ai-tool-status-icon="failed"]')).toBeInTheDocument()
    expect(container.querySelector('[data-ai-tool-status-icon="running"]')).not.toBeInTheDocument()
  })

  it('shows a rejected approval as rejected instead of completed', async () => {
    await i18next.changeLanguage('zh-CN')
    const { container } = render(
      <AIToolCallCard
        block={toolBlock('rejected')}
        onAction={vi.fn(async () => true)}
        onApproval={vi.fn(async () => {})}
      />,
    )

    const summary = container.querySelector('[data-ai-tool-summary]')
    expect(summary).toHaveTextContent('已拒绝')
    expect(summary).not.toHaveTextContent('已完成')
    expect(container.querySelector('[data-ai-tool-status-icon="rejected"]')).toBeInTheDocument()
  })

  it('keeps the collapsed row compact and exposes copy controls in expanded details', async () => {
    const writeText = vi.fn(async () => {})
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    const { container } = render(
      <AIToolCallCard
        block={{
          ...toolBlock('succeeded'),
          durationMs: 128,
          traceId: '717690e2661f8337d53fcd3295591b4b',
          arguments: {
            projectId: 'prj_1',
            filters: { status: ['failed', 'running'], limit: 20 },
          },
          result: { summaryKey: 'aiAssistant.resultAvailable' },
        }}
        onAction={vi.fn(async () => true)}
        onApproval={vi.fn(async () => {})}
      />,
    )

    const details = container.querySelector('details')
    const summary = container.querySelector('[data-ai-tool-summary]')
    expect(summary).toHaveTextContent('查询应用列表')
    expect(summary).not.toHaveTextContent('listApplications')
    expect(summary).toHaveTextContent('已完成')
    expect(summary).toHaveTextContent('·')
    expect(summary).toHaveTextContent('128 ms')
    expect(summary?.querySelector('[data-ai-tool-duration]')).toHaveTextContent('128 ms')
    expect(summary).not.toHaveTextContent('工具已返回结果')
    expect(details).not.toHaveAttribute('open')

    fireEvent.click(summary!)

    expect(details).toHaveAttribute('open')
    expect(screen.getByText('标识')).toBeInTheDocument()
    expect(screen.getByText('工具标识')).toBeInTheDocument()
    expect(screen.getByText('listApplications')).toBeInTheDocument()
    expect(screen.getByText('调用标识')).toBeInTheDocument()
    expect(screen.getByText('tool-call-1')).toBeInTheDocument()
    expect(screen.getByText('运行标识')).toBeInTheDocument()
    expect(screen.getByText('run-1')).toBeInTheDocument()
    expect(screen.getByText('Trace ID')).toBeInTheDocument()
    expect(screen.getByText('717690e2661f8337d53fcd3295591b4b')).toBeInTheDocument()
    expect(screen.getByText('projectId')).toBeInTheDocument()
    expect(screen.getByText(/"failed"/)).toBeInTheDocument()
    expect(screen.getByText('返回')).toBeInTheDocument()
    expect(screen.getByText('工具已返回结果')).toBeInTheDocument()
    expect(screen.getByText('耗时')).toBeInTheDocument()
    expect(screen.getAllByText('128 ms')).toHaveLength(2)

    fireEvent.click(screen.getByRole('button', { name: '复制 Trace ID' }))
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('717690e2661f8337d53fcd3295591b4b'))

    fireEvent.click(screen.getByRole('button', { name: '复制' }))
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(expect.stringContaining('"status"')))
  })

  it('keeps a running status compact when no duration is available', async () => {
    await i18next.changeLanguage('zh-CN')
    const { container } = render(
      <AIToolCallCard
        block={toolBlock('running')}
        onAction={vi.fn(async () => true)}
        onApproval={vi.fn(async () => {})}
      />,
    )

    const summary = container.querySelector('[data-ai-tool-summary]')
    expect(summary).toHaveTextContent('进行中')
    expect(summary).not.toHaveTextContent('·')
  })

  it('shows a safe failure reason and request id without exposing backend details', async () => {
    await i18next.changeLanguage('zh-CN')
    render(
      <AIToolCallCard
        block={{
          ...toolBlock('failed'),
          errorCode: 'ai.tool_storage_unavailable',
          result: {
            summaryKey: 'ai.tool.result.completed',
            requestId: 'req_tool_failure',
            errorCode: 'ai.tool_storage_unavailable',
          },
        }}
        onAction={vi.fn(async () => true)}
        onApproval={vi.fn(async () => {})}
      />,
    )

    fireEvent.click(screen.getByText('查询应用列表'))
    expect(screen.getByText('平台数据暂时无法读取，请稍后重试；如持续失败，请提供请求编号。')).toBeInTheDocument()
    expect(screen.getByText('请求编号')).toBeInTheDocument()
    expect(screen.getByText('req_tool_failure')).toBeInTheDocument()
    expect(screen.queryByText(/select .* from/i)).not.toBeInTheDocument()
  })

  it('renders bounded non-sensitive response data and card validation issues', async () => {
    await i18next.changeLanguage('zh-CN')
    render(
      <AIToolCallCard
        block={{
          ...toolBlock('failed'),
          operationId: 'present_card',
          errorCode: 'ai.provider_invalid_tool_arguments',
          result: {
            summaryKey: 'aiAssistant.cards.failed',
            errorCode: 'ai.provider_invalid_tool_arguments',
            data: { items: [{ id: 'app-1', name: 'PostgreSQL' }], total: 1 },
            issues: [{
              code: 'too_small',
              path: 'cards.0.sections',
              message: 'Too small: expected array to have >=1 items',
            }],
          },
        }}
        onAction={vi.fn(async () => true)}
        onApproval={vi.fn(async () => {})}
      />,
    )

    fireEvent.click(screen.getByText('展示信息卡片'))
    expect(screen.getByText('模型生成的工具参数不符合要求，请查看校验详情。')).toBeInTheDocument()
    expect(screen.getByText('cards.0.sections')).toBeInTheDocument()
    expect(screen.getByText('Too small: expected array to have >=1 items')).toBeInTheDocument()
    expect(screen.getByText(/"name": "PostgreSQL"/)).toBeInTheDocument()
  })

  it('offers reject and one-time approve decisions for a high-risk call', async () => {
    await i18next.changeLanguage('zh-CN')
    const onApproval = vi.fn(async () => {})
    const { container } = render(
      <AIToolCallCard
        block={{
          ...toolBlock('awaiting_approval'),
          arguments: { applicationId: 'app-1' },
        }}
        onAction={vi.fn(async () => true)}
        onApproval={onApproval}
      />,
    )

    expect(container.querySelector('details')).not.toHaveAttribute('open')
    expect(container.querySelector('[data-ai-tool-intervention]')).toBeVisible()
    expect(container.querySelector('[data-ai-approval-parameters]')).toHaveTextContent('应用')
    expect(container.querySelector('[data-ai-approval-parameters]')).toHaveTextContent('app-1')
    expect(screen.getByRole('button', { name: '拒绝' })).toBeVisible()
    expect(screen.getByRole('button', { name: '批准执行' })).toBeVisible()
    expect(screen.queryByRole('button', { name: '批准并在本会话记住' })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '批准执行' }))
    await waitFor(() => expect(onApproval).toHaveBeenCalledWith(
      expect.objectContaining({ runId: 'run-1', toolCallId: 'tool-call-1' }),
      'approve',
    ))
  })

  it('offers session-remembered approval alongside one-time approve for L1 reversible calls', async () => {
    await i18next.changeLanguage('zh-CN')
    const onApproval = vi.fn(async () => {})
    render(
      <AIToolCallCard
        block={{
          ...toolBlock('awaiting_approval'),
          approvalTier: 'L1',
          approvalRememberable: true,
          arguments: { projectId: 'prj_1', body: { stage: 'dev' } },
        }}
        onAction={vi.fn(async () => true)}
        onApproval={onApproval}
      />,
    )

    expect(screen.getByRole('button', { name: '拒绝' })).toBeVisible()
    expect(screen.getByRole('button', { name: '批准执行' })).toBeVisible()
    const rememberBtn = screen.getByRole('button', { name: '批准并在本会话记住' })
    expect(rememberBtn).toBeVisible()

    fireEvent.click(rememberBtn)
    await waitFor(() => expect(onApproval).toHaveBeenCalledWith(
      expect.objectContaining({ runId: 'run-1', toolCallId: 'tool-call-1', approvalTier: 'L1' }),
      'approve_and_remember',
    ))
  })

  it('renders all approval parameters without dropping fields beyond 5 entries and prevents duplicate synchronous clicks', async () => {
    await i18next.changeLanguage('zh-CN')
    let resolveApproval!: () => void
    const onApproval = vi.fn(() => new Promise<void>((resolve) => {
      resolveApproval = resolve
    }))
    const { container } = render(
      <AIToolCallCard
        block={{
          ...toolBlock('awaiting_approval'),
          approvalTier: 'L2',
          approvalRememberable: false,
          arguments: {
            projectId: 'prj_99',
            templateId: 'tpl_pg',
            body: {
              applicationName: 'Core DB',
              applicationIdentifier: 'core-db',
              deploymentName: 'core-db-prod',
              clusterId: 'cls_prod_1',
              stage: 'prod',
              command: 'rm -rf /var/lib/postgresql/data',
            },
          },
        }}
        onAction={vi.fn(async () => true)}
        onApproval={onApproval}
      />,
    )

    const paramsPanel = container.querySelector('[data-ai-approval-parameters]')
    expect(paramsPanel).not.toBeNull()
    for (const expectedText of [
      'prj_99',
      'tpl_pg',
      'Core DB',
      'core-db',
      'core-db-prod',
      'cls_prod_1',
      'prod',
      'rm -rf /var/lib/postgresql/data',
    ]) {
      expect(paramsPanel).toHaveTextContent(expectedText)
    }

    const approveBtn = screen.getByRole('button', { name: '批准执行' })
    fireEvent.click(approveBtn)
    fireEvent.click(approveBtn)
    expect(onApproval).toHaveBeenCalledTimes(1)
    resolveApproval()
    await waitFor(() => expect(approveBtn).not.toBeDisabled())
  })

  it('does not show approval controls for an ordinary tool call', () => {
    const { container } = render(
      <AIToolCallCard
        block={toolBlock('running')}
        onAction={vi.fn(async () => true)}
        onApproval={vi.fn(async () => {})}
      />,
    )

    expect(container.querySelector('[data-ai-tool-intervention]')).not.toBeInTheDocument()
  })

  it('renders wait_for_task visibly with target task label, live progress while running, and terminal taskStatus badge', async () => {
    await i18next.changeLanguage('zh-CN')
    const { container, rerender } = render(
      <AIToolCallCard
        block={{
          ...toolBlock('running'),
          operationId: 'wait_for_task',
          visibility: 'normal',
          arguments: { projectId: 'prj_1', taskType: 'build_run', taskId: 'bldr_99' },
        }}
        onAction={vi.fn(async () => true)}
        onApproval={vi.fn(async () => {})}
      />,
    )

    expect(container.querySelector('[data-ai-tool-summary]')).toHaveTextContent('等待后台任务完成 · 构建 bldr_99')
    expect(container.querySelector('[data-ai-wait-for-task-progress]')).not.toBeNull()

    rerender(
      <AIToolCallCard
        block={{
          ...toolBlock('succeeded'),
          operationId: 'wait_for_task',
          visibility: 'normal',
          arguments: { projectId: 'prj_1', taskType: 'build_run', taskId: 'bldr_99' },
          result: {
            summaryKey: 'aiAssistant.resultAvailable',
            data: { taskStatus: 'failed', timedOut: false },
          },
        }}
        onAction={vi.fn(async () => true)}
        onApproval={vi.fn(async () => {})}
      />,
    )

    expect(container.querySelector('[data-ai-wait-for-task-progress]')).toBeNull()
    expect(container.querySelector('[data-ai-tool-summary]')).toHaveTextContent('失败')
  })

  it('[P2-3] rejects invalid path-segment projectId or taskId in wait_for_task live progress binding', async () => {
    await i18next.changeLanguage('zh-CN')
    const { container } = render(
      <AIToolCallCard
        block={{
          ...toolBlock('running'),
          operationId: 'wait_for_task',
          visibility: 'normal',
          arguments: { projectId: '../etc', taskType: 'build_run', taskId: 'bldr_99' },
        }}
        onAction={vi.fn(async () => true)}
        onApproval={vi.fn(async () => {})}
      />,
    )
    expect(container.querySelector('[data-ai-wait-for-task-progress]')).toBeNull()
  })

  it('renders L2 pre-approval diff preview, sensitive key names, and warnings', async () => {
    await i18next.changeLanguage('zh-CN')
    const { container } = render(
      <AIToolCallCard
        block={{
          ...toolBlock('awaiting_approval'),
          operationId: 'delivery_configure_target_config',
          approvalTier: 'L2',
          approvalRememberable: false,
          approvalPreview: {
            summary: '更新 prod 环境配置（需重新发布生效）',
            changes: [
              { field: 'replicas', before: 2, after: 4, action: 'update' },
            ],
            sensitiveKeys: ['DATABASE_PASSWORD'],
            impact: ['影响 1 个部署环境 (prod)'],
            warnings: ['变更将在下次发布后下发 Kubernetes'],
          },
          arguments: { project: 'prj_1', application: 'api', target: 'prod' },
        }}
        onAction={vi.fn(async () => true)}
        onApproval={vi.fn(async () => {})}
      />,
    )

    expect(container.querySelector('[data-ai-approval-preview-summary]')).toHaveTextContent('更新 prod 环境配置（需重新发布生效）')
    expect(container.querySelector('[data-ai-approval-diff]')).toHaveTextContent('2 -> 4')
    expect(container.querySelector('[data-ai-approval-diff]')).toHaveTextContent('副本数')
    expect(container.querySelector('[data-ai-approval-sensitive-keys]')).toHaveTextContent('DATABASE_PASSWORD')
    expect(container.querySelector('[data-ai-approval-impact]')).toHaveTextContent('影响 1 个部署环境 (prod)')
    expect(container.querySelector('[data-ai-approval-warnings]')).toHaveTextContent('变更将在下次发布后下发 Kubernetes')
    expect(container.querySelector('[data-ai-approval-scope]')).toHaveTextContent('prj_1')
    expect(container.querySelector('[data-ai-approval-scope]')).toHaveTextContent('api')
    expect(container.querySelector('details[data-ai-approval-parameters]')).not.toHaveAttribute('open')
  })

  it('keeps target project scope visible by default even when diff changes collapse raw parameters', async () => {
    await i18next.changeLanguage('zh-CN')
    const { container } = render(
      <AIToolCallCard
        block={{
          ...toolBlock('awaiting_approval'),
          operationId: 'build_configure_target_pipeline',
          approvalTier: 'L2',
          approvalRememberable: false,
          approvalPreview: {
            summary: '',
            targetResource: 'build_environment_config',
            targetId: 'clu_89e66ebb87c11976a0d33667',
            projectId: 'prj_33d6a910384769b3b35984e5',
            changes: [{ field: 'autoDeploy', before: false, after: true, action: 'update' }],
          },
          arguments: {
            project: 'prj_33d6a910384769b3b35984e5',
            cluster: 'clu_89e66ebb87c11976a0d33667',
            autoDeploy: true,
          },
        }}
        onAction={vi.fn(async () => true)}
        onApproval={vi.fn(async () => {})}
      />,
    )

    expect(container.querySelector('[data-ai-tool-summary]')).not.toHaveTextContent('build_environment_config')
    expect(container.querySelector('details[data-ai-approval-parameters]')).not.toHaveAttribute('open')
    const scopeBox = container.querySelector<HTMLElement>('[data-ai-approval-scope]')!
    expect(scopeBox).toHaveTextContent('项目空间')
    expect(scopeBox).toHaveTextContent('prj_33d6…984e5')
    expect(scopeBox).toHaveTextContent('clu_89e6…33667')
  })

  it('keeps L1 strict-mode calls labeled as standard changes, humanizes settings, and collapses technical details by default', async () => {
    await i18next.changeLanguage('zh-CN')
    const { container } = render(
      <AIToolCallCard
        block={{
          ...toolBlock('awaiting_approval'),
          operationId: 'build_configure_target_pipeline',
          approvalTier: 'L1',
          approvalRememberable: false,
          approvalPreview: {
            summary: '',
            targetResource: 'deployment_target',
            warnings: ['pipeline_applies_to_next_build'],
          },
          arguments: {
            project: 'prj_33d6a910384769b3b35984e5',
            target: 'dplt_89e66ebb87c11976a0d33667',
            targetRegistry: 'reg_5ba728de4e545c5bda7a310f',
            application: 'blog-xingluo',
            identifier: 'app_backend',
            autoDeploy: true,
            branchPattern: 'main',
          },
        }}
        onAction={vi.fn(async () => true)}
        onApproval={vi.fn(async () => {})}
      />,
    )

    expect(container.querySelector('[data-ai-tool-summary]')).toHaveTextContent('配置构建流水线 · blog-xingluo')
    expect(container.querySelector('[data-ai-tool-summary]')).not.toHaveTextContent('deployment_target')
    expect(container.querySelector('[data-ai-approval-tier="L1"]')).not.toBeNull()
    expect(screen.getByText('请确认变更内容')).toBeInTheDocument()
    expect(screen.getByText('常规变更')).toBeInTheDocument()
    expect(screen.getByText('当前会话开启了严格确认模式，请核对以下内容后执行。')).toBeInTheDocument()
    expect(screen.getByText('流水线配置将在下一次触发构建时生效。')).toBeInTheDocument()
    const paramsPanel = container.querySelector<HTMLElement>('[data-ai-approval-parameters]')!
    expect(paramsPanel).toHaveTextContent('自动发布')
    expect(paramsPanel).toHaveTextContent('开启')
    expect(paramsPanel).toHaveTextContent('标识')
    expect(paramsPanel).toHaveTextContent('app_backend')
    // 24-hex scope/target IDs remain visible in compact form so target project/resource cannot be hidden
    expect(paramsPanel).toHaveTextContent('prj_33d6…984e5')
    expect(container.querySelector('[data-ai-tool-technical-details]')).not.toHaveAttribute('open')
  })
})
