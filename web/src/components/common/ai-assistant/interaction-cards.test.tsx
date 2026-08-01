import type { AIUIAction } from '@/api'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AIInteractionCards } from './interaction-cards'

const catalogCard = {
  schemaVersion: 1,
  generationId: 'database-candidates',
  title: '选择数据库',
  mode: 'interactive',
  template: 'catalog',
  cards: [{
    id: 'postgresql',
    presentation: {
      variant: 'application',
      title: 'PostgreSQL',
      description: '可靠的关系型数据库',
      icon: { type: 'category', name: 'database', alt: 'PostgreSQL' },
    },
    blocks: [{
      id: 'facts',
      type: 'key_value',
      items: [{ label: '版本', value: '16', format: 'code' }],
    }],
    form: {
      sections: [{
        id: 'target',
        fields: [
          {
            id: 'projectId',
            type: 'select',
            label: '项目空间',
            required: true,
            submissionFormat: 'label_value',
            options: [{ value: 'prj_example', label: '示例项目空间' }],
            defaultValue: 'prj_example',
          },
          {
            id: 'applicationName',
            type: 'text',
            label: '应用名称',
            required: true,
            defaultValue: 'postgresql',
          },
          {
            id: 'password',
            type: 'secret',
            label: '数据库密码',
            required: true,
            generation: 'required',
          },
        ],
      }],
    },
    actions: [{
      id: 'install',
      type: 'tool',
      label: '安装 PostgreSQL',
      emphasis: 'primary',
      operationId: 'installAppTemplate',
      bindings: [
        { target: '/projectId', value: { type: 'field', fieldId: 'projectId' } },
        { target: '/applicationName', value: { type: 'field', fieldId: 'applicationName' } },
        { target: '/password', value: { type: 'field', fieldId: 'password' } },
        { target: '/templateId', value: { type: 'literal', value: 'postgresql' } },
      ],
    }],
  }],
} as const

describe('ai interaction cards', () => {
  it('binds safe form values to a tool request and omits secret fields', async () => {
    const onAction = vi.fn<(action: AIUIAction) => Promise<boolean>>().mockResolvedValue(true)
    render(<AIInteractionCards arguments={catalogCard} onAction={onAction} />)

    const actionButton = screen.getByRole('button', { name: '安装 PostgreSQL' })
    await waitFor(() => expect(actionButton).toBeEnabled())
    fireEvent.click(actionButton)

    await waitFor(() => expect(onAction).toHaveBeenCalledOnce())
    expect(onAction).toHaveBeenCalledWith(expect.objectContaining({
      type: 'request_tool',
      payload: {
        operationId: 'installAppTemplate',
        arguments: {
          projectId: 'prj_example',
          applicationName: 'postgresql',
          templateId: 'postgresql',
        },
        message: '安装 PostgreSQL',
      },
    }))
  })

  it('keeps a tool action disabled until required fields are valid', async () => {
    const card = structuredClone(catalogCard) as unknown as {
      cards: Array<{ form: { sections: Array<{ fields: Array<{ id: string, defaultValue?: string }> }> } }>
    }
    const nameField = card.cards[0]!.form.sections[0]!.fields.find(field => field.id === 'applicationName')
    delete nameField!.defaultValue
    render(<AIInteractionCards arguments={card as unknown as Record<string, unknown>} onAction={vi.fn()} />)

    const actionButton = screen.getByRole('button', { name: '安装 PostgreSQL' })
    expect(actionButton).toBeDisabled()
    fireEvent.change(screen.getByLabelText('应用名称 *'), { target: { value: 'postgresql' } })
    await waitFor(() => expect(actionButton).toBeEnabled())
  })

  it('validates and expands non-sensitive form fields in a send-message action', async () => {
    const onAction = vi.fn<(action: AIUIAction) => Promise<boolean>>().mockResolvedValue(true)
    const card = structuredClone(catalogCard) as unknown as {
      cards: Array<{
        form: { sections: Array<{ fields: Array<{ id: string, defaultValue?: string }> }> }
        actions: unknown[]
      }>
    }
    const nameField = card.cards[0]!.form.sections[0]!.fields.find(field => field.id === 'applicationName')
    delete nameField!.defaultValue
    card.cards[0]!.actions = [{
      id: 'continue',
      type: 'send_message',
      label: '继续配置',
      emphasis: 'primary',
      message: '继续配置 {{applicationName}}，目标是 {{projectId}}。',
    }]
    render(<AIInteractionCards arguments={card as unknown as Record<string, unknown>} onAction={onAction} />)

    const actionButton = screen.getByRole('button', { name: '继续配置' })
    expect(actionButton).toBeDisabled()
    fireEvent.change(screen.getByLabelText('应用名称 *'), { target: { value: 'redis-cache' } })
    await waitFor(() => expect(actionButton).toBeEnabled())
    fireEvent.click(actionButton)

    await waitFor(() => expect(onAction).toHaveBeenCalledWith(expect.objectContaining({
      type: 'send_message',
      payload: { message: '继续配置 redis-cache，目标是 示例项目空间 (prj_example)。' },
    })))
  })

  it('formats every selected resource name and ID in a multi-select reply', async () => {
    const onAction = vi.fn<(action: AIUIAction) => Promise<boolean>>().mockResolvedValue(true)
    const card = {
      schemaVersion: 1,
      generationId: 'cluster-selection',
      title: '选择集群',
      mode: 'interactive',
      template: 'form',
      cards: [{
        id: 'clusters',
        presentation: { variant: 'form', title: '目标集群' },
        form: {
          sections: [{
            id: 'selection',
            fields: [{
              id: 'clusterIds',
              type: 'multi_select',
              label: '集群',
              required: true,
              submissionFormat: 'label_value',
              options: [
                { value: 'clu_chongqing', label: '重庆集群' },
                { value: 'clu_shanghai', label: '上海集群' },
              ],
            }],
          }],
        },
        actions: [{
          id: 'continue',
          type: 'send_message',
          label: '确认集群',
          message: '使用 {{clusterIds}}。',
          emphasis: 'primary',
        }],
      }],
    }
    render(<AIInteractionCards arguments={card} onAction={onAction} />)

    fireEvent.click(screen.getByRole('checkbox', { name: /重庆集群/ }))
    fireEvent.click(screen.getByRole('checkbox', { name: /上海集群/ }))
    await waitFor(() => expect(screen.getByRole('button', { name: '确认集群' })).toBeEnabled())
    fireEvent.click(screen.getByRole('button', { name: '确认集群' }))

    await waitFor(() => expect(onAction).toHaveBeenCalledWith(expect.objectContaining({
      type: 'send_message',
      payload: { message: '使用 重庆集群 (clu_chongqing)、上海集群 (clu_shanghai)。' },
    })))
  })

  it('fails closed for an invalid model-generated card payload', () => {
    render(<AIInteractionCards arguments={{ schemaVersion: 1, template: 'approval', cards: [] }} onAction={vi.fn()} />)

    expect(screen.getByRole('alert')).toBeInTheDocument()
  })

  it('rejects a display-only candidate list when the workflow is waiting for a selection', () => {
    render(
      <AIInteractionCards
        arguments={{
          schemaVersion: 1,
          generationId: 'template-selection',
          title: '请选择应用模板',
          mode: 'interactive',
          template: 'catalog',
          cards: [{
            id: 'templates',
            presentation: { variant: 'application', title: '应用模板市场' },
            blocks: [{
              id: 'template-list',
              type: 'item_list',
              items: [
                { id: 'postgresql', primary: 'PostgreSQL' },
                { id: 'redis', primary: 'Redis' },
              ],
            }],
          }],
        }}
        onAction={vi.fn()}
      />,
    )

    expect(screen.getByRole('alert')).toHaveTextContent('aiAssistant.cards.invalid')
    expect(screen.queryByText('PostgreSQL')).not.toBeInTheDocument()
  })

  it('submits a candidate selected through an interactive form card', async () => {
    const onAction = vi.fn<(action: AIUIAction) => Promise<boolean>>().mockResolvedValue(true)
    const card = {
      schemaVersion: 1,
      generationId: 'template-selection',
      title: '选择应用模板',
      mode: 'interactive',
      template: 'form',
      cards: [{
        id: 'template-selection',
        presentation: { variant: 'form', title: '应用模板' },
        form: {
          sections: [{
            id: 'selection',
            fields: [{
              id: 'templateId',
              type: 'select',
              display: 'radio',
              label: '选择模板',
              required: true,
              options: [
                { value: 'postgresql', label: 'PostgreSQL', description: '关系型数据库' },
                { value: 'redis', label: 'Redis', description: '内存缓存' },
              ],
            }],
          }],
        },
        actions: [{
          id: 'continue',
          type: 'send_message',
          label: '继续配置',
          message: '继续配置 {{templateId}}。',
          emphasis: 'primary',
        }],
      }],
    }
    render(<AIInteractionCards arguments={card} onAction={onAction} />)

    const actionButton = screen.getByRole('button', { name: '继续配置' })
    expect(actionButton).toBeDisabled()
    fireEvent.click(screen.getByRole('radio', { name: /PostgreSQL/ }))
    await waitFor(() => expect(actionButton).toBeEnabled())
    fireEvent.click(actionButton)

    await waitFor(() => expect(onAction).toHaveBeenCalledWith(expect.objectContaining({
      type: 'send_message',
      payload: { message: '继续配置 postgresql。' },
    })))
  })

  it('renders card descriptions as safe markdown and ignores model HTML', () => {
    const card = structuredClone(catalogCard) as unknown as {
      description?: string
      cards: Array<{ presentation: { description?: string } }>
    }
    card.description = '请选择 **可信来源** 的数据库。'
    card.cards[0]!.presentation.description = '适合生产环境。<script>window.bad = true</script>'

    const { container } = render(<AIInteractionCards arguments={card as unknown as Record<string, unknown>} onAction={vi.fn()} />)

    expect(screen.getByText('可信来源').tagName).toBe('STRONG')
    expect(container.querySelector('script')).not.toBeInTheDocument()
  })
})
