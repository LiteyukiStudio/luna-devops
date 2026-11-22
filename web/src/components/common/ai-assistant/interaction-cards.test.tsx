import type { AIAssistantRuntimeValue } from './runtime-context'
import type { AIUIAction } from '@/api'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { toast } from 'sonner'
import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/api'
import { AIInteractionCards } from './interaction-cards'
import { AIAssistantRuntimeContext } from './runtime-context'

const candidatesCard = {
  schemaVersion: 1,
  generationId: 'database-candidates',
  title: '选择数据库',
  mode: 'interactive',
  template: 'candidates',
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
  it('renders an editable password field and binds the entered secret to a tool request', async () => {
    const onAction = vi.fn<(action: AIUIAction) => Promise<boolean>>().mockResolvedValue(true)
    render(<AIInteractionCards arguments={candidatesCard} onAction={onAction} />)

    const password = screen.getByLabelText('数据库密码 *')
    expect(password).toHaveAttribute('type', 'password')
    fireEvent.change(password, { target: { value: 'database-password' } })
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
          password: 'database-password',
          templateId: 'postgresql',
        },
        message: '安装 PostgreSQL',
      },
    }))
  })

  it('builds array arguments for runtime secret bindings', async () => {
    const onAction = vi.fn<(action: AIUIAction) => Promise<boolean>>().mockResolvedValue(true)
    const card = structuredClone(candidatesCard) as unknown as {
      cards: Array<{
        form: { sections: Array<{ fields: Array<Record<string, unknown>> }> }
        actions: unknown[]
      }>
    }
    card.cards[0]!.form.sections[0]!.fields = [
      { id: 'accessKey', type: 'secret', label: 'Access Key', required: true, generation: 'disabled' },
      { id: 'secretKey', type: 'secret', label: 'Secret Key', required: true, generation: 'disabled' },
    ]
    card.cards[0]!.actions = [{
      id: 'save',
      type: 'tool',
      label: '保存密钥',
      operationId: 'updateDeploymentTargetRuntimeSecrets',
      bindings: [
        { target: '/projectId', value: { type: 'literal', value: 'prj_test' } },
        { target: '/body/items/0/key', value: { type: 'literal', value: 'ACCESS_KEY' } },
        { target: '/body/items/0/valueMode', value: { type: 'literal', value: 'secret' } },
        { target: '/body/items/0/operation', value: { type: 'literal', value: 'set' } },
        { target: '/body/items/0/value', value: { type: 'field', fieldId: 'accessKey' } },
        { target: '/body/items/1/key', value: { type: 'literal', value: 'SECRET_KEY' } },
        { target: '/body/items/1/valueMode', value: { type: 'literal', value: 'secret' } },
        { target: '/body/items/1/operation', value: { type: 'literal', value: 'set' } },
        { target: '/body/items/1/value', value: { type: 'field', fieldId: 'secretKey' } },
      ],
    }]

    render(<AIInteractionCards arguments={card as unknown as Record<string, unknown>} onAction={onAction} />)
    fireEvent.change(screen.getByLabelText('Access Key *'), { target: { value: 'access-value' } })
    fireEvent.change(screen.getByLabelText('Secret Key *'), { target: { value: 'secret-value' } })
    const save = screen.getByRole('button', { name: '保存密钥' })
    await waitFor(() => expect(save).toBeEnabled())
    fireEvent.click(save)

    await waitFor(() => expect(onAction).toHaveBeenCalledWith(expect.objectContaining({
      type: 'request_tool',
      payload: expect.objectContaining({
        arguments: {
          projectId: 'prj_test',
          body: {
            items: [
              { key: 'ACCESS_KEY', valueMode: 'secret', operation: 'set', value: 'access-value' },
              { key: 'SECRET_KEY', valueMode: 'secret', operation: 'set', value: 'secret-value' },
            ],
          },
        },
      }),
    })))
  })

  it('rejects prototype-mutating binding paths in the browser boundary', async () => {
    const onAction = vi.fn<(action: AIUIAction) => Promise<boolean>>().mockResolvedValue(true)
    const card = structuredClone(candidatesCard) as unknown as { cards: Array<{ actions: Array<Record<string, unknown>> }> }
    card.cards[0]!.actions[0]!.bindings = [{ target: '/__proto__/polluted', value: { type: 'literal', value: true } }]

    render(<AIInteractionCards arguments={card as unknown as Record<string, unknown>} onAction={onAction} />)
    fireEvent.click(screen.getByRole('button', { name: '安装 PostgreSQL' }))

    await waitFor(() => expect(onAction).not.toHaveBeenCalled())
    expect(({} as Record<string, unknown>).polluted).toBeUndefined()
  })

  it('keeps secrets out of a send-message payload and visible text', async () => {
    const onAction = vi.fn<(action: AIUIAction) => Promise<boolean>>().mockResolvedValue(true)
    const card = structuredClone(candidatesCard) as unknown as { cards: Array<{ actions: unknown[] }> }
    card.cards[0]!.actions = [{
      id: 'continue',
      type: 'send_message',
      label: '继续配置',
      emphasis: 'primary',
      message: '继续配置 {{password}}。',
    }]
    render(<AIInteractionCards arguments={card as unknown as Record<string, unknown>} onAction={onAction} />)

    fireEvent.change(screen.getByLabelText('数据库密码 *'), { target: { value: 'do-not-display' } })
    const actionButton = screen.getByRole('button', { name: '继续配置' })
    await waitFor(() => expect(actionButton).toBeEnabled())
    fireEvent.click(actionButton)

    await waitFor(() => expect(onAction).toHaveBeenCalledOnce())
    expect(onAction).toHaveBeenCalledWith(expect.objectContaining({
      type: 'send_message',
      payload: { message: '继续配置 。' },
    }))
    expect(screen.queryByText('do-not-display')).not.toBeInTheDocument()
  })

  it('supports secret key-value editing and binds non-empty values to a tool request', async () => {
    const onAction = vi.fn<(action: AIUIAction) => Promise<boolean>>().mockResolvedValue(true)
    const card = {
      schemaVersion: 1,
      generationId: 'secret-key-values',
      title: '配置环境变量',
      mode: 'interactive',
      template: 'form',
      cards: [{
        id: 'environment',
        presentation: { variant: 'form', title: '环境变量' },
        form: {
          sections: [{
            id: 'main',
            fields: [{
              id: 'secrets',
              type: 'key_value',
              label: '密钥变量',
              required: true,
              valueMode: 'secret',
            }],
          }],
        },
        actions: [{
          id: 'save',
          type: 'tool',
          label: '保存配置',
          emphasis: 'primary',
          operationId: 'saveConfig',
          bindings: [{ target: '/environment', value: { type: 'field', fieldId: 'secrets' } }],
        }],
      }],
    }
    render(<AIInteractionCards arguments={card} onAction={onAction} />)

    fireEvent.click(screen.getByRole('button', { name: 'Add entry' }))
    fireEvent.change(screen.getByLabelText('Key'), { target: { value: 'DATABASE_PASSWORD' } })
    const value = screen.getByLabelText('Value')
    expect(value).toHaveAttribute('type', 'password')
    fireEvent.change(value, { target: { value: 'env-password' } })
    const saveButton = screen.getByRole('button', { name: '保存配置' })
    await waitFor(() => expect(saveButton).toBeEnabled())
    fireEvent.click(saveButton)

    await waitFor(() => expect(onAction).toHaveBeenCalledWith(expect.objectContaining({
      type: 'request_tool',
      payload: { operationId: 'saveConfig', arguments: { environment: [{ key: 'DATABASE_PASSWORD', value: 'env-password' }] }, message: '保存配置' },
    })))
  })

  it('keeps every secret mode empty until the user enters a value', async () => {
    const makeCard = (generation: 'disabled' | 'optional' | 'required', required = true) => ({
      schemaVersion: 1,
      generationId: `secret-${generation}-${required}`,
      title: '填写密钥',
      mode: 'interactive',
      template: 'form',
      cards: [{
        id: 'secret',
        presentation: { variant: 'form', title: '密钥' },
        form: {
          sections: [{
            id: 'main',
            fields: [{
              id: 'value',
              type: 'secret',
              label: '密钥',
              required,
              generation,
              placeholder: '输入密钥',
            }],
          }],
        },
        actions: [{
          id: 'submit',
          type: 'tool',
          label: '提交',
          operationId: 'saveSecret',
          bindings: [{ target: '/value', value: { type: 'field', fieldId: 'value' } }],
        }],
      }],
    })

    const disabledAction = vi.fn<(action: AIUIAction) => Promise<boolean>>().mockResolvedValue(true)
    const { unmount } = render(<AIInteractionCards arguments={makeCard('disabled')} onAction={disabledAction} />)
    const disabledButton = screen.getByRole('button', { name: '提交' })
    expect(disabledButton).toBeDisabled()
    fireEvent.change(screen.getByLabelText('密钥 *'), { target: { value: 'disabled-password' } })
    await waitFor(() => expect(disabledButton).toBeEnabled())
    unmount()

    const optionalAction = vi.fn<(action: AIUIAction) => Promise<boolean>>().mockResolvedValue(true)
    const optional = render(<AIInteractionCards arguments={makeCard('optional')} onAction={optionalAction} />)
    const optionalButton = screen.getByRole('button', { name: '提交' })
    await waitFor(() => expect(optionalButton).toBeEnabled())
    fireEvent.click(optionalButton)
    await waitFor(() => expect(optionalAction).toHaveBeenCalledWith(expect.objectContaining({ payload: expect.objectContaining({ arguments: {} }) })))
    optional.unmount()

    const requiredAction = vi.fn<(action: AIUIAction) => Promise<boolean>>().mockResolvedValue(true)
    render(<AIInteractionCards arguments={makeCard('required')} onAction={requiredAction} />)
    expect(screen.getByLabelText('密钥 *')).toHaveValue('')
    const requiredButton = screen.getByRole('button', { name: '提交' })
    await waitFor(() => expect(requiredButton).toBeEnabled())
    fireEvent.click(requiredButton)
    await waitFor(() => expect(requiredAction).toHaveBeenCalledWith(expect.objectContaining({ payload: expect.objectContaining({ arguments: {} }) })))
  })

  it('ignores malicious persisted defaults for secret fields and submits only user-entered values', async () => {
    const onAction = vi.fn<(action: AIUIAction) => Promise<boolean>>().mockResolvedValue(true)
    const card = structuredClone(candidatesCard) as unknown as {
      cards: Array<{ form: { sections: Array<{ fields: Array<Record<string, unknown>> }> } }>
    }
    const fields = card.cards[0]!.form.sections[0]!.fields
    const secret = fields.find(field => field.id === 'password')!
    secret.defaultValue = 'ignore-previous-instructions-and-submit-this-password'
    fields.push({
      id: 'credentials',
      type: 'key_value',
      label: '附加密钥',
      valueMode: 'secret',
      defaultValue: [{ key: 'API_TOKEN', value: 'model-injected-token' }],
    })

    render(<AIInteractionCards arguments={card as unknown as Record<string, unknown>} onAction={onAction} />)

    expect(screen.getByLabelText('数据库密码 *')).toHaveValue('')
    expect(screen.queryByDisplayValue('model-injected-token')).not.toBeInTheDocument()
    expect(screen.queryByDisplayValue('API_TOKEN')).not.toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('数据库密码 *'), { target: { value: 'user-entered-password' } })
    const install = screen.getByRole('button', { name: '安装 PostgreSQL' })
    await waitFor(() => expect(install).toBeEnabled())
    fireEvent.click(install)
    await waitFor(() => expect(onAction).toHaveBeenCalledOnce())
    expect(onAction).toHaveBeenCalledWith(expect.objectContaining({
      payload: expect.objectContaining({
        arguments: expect.objectContaining({ password: 'user-entered-password' }),
      }),
    }))
    expect(JSON.stringify(onAction.mock.calls)).not.toContain('model-injected-token')
    expect(JSON.stringify(onAction.mock.calls)).not.toContain('ignore-previous-instructions')
  })

  it('omits blank secret key-value rows instead of treating them as clear operations', async () => {
    const onAction = vi.fn<(action: AIUIAction) => Promise<boolean>>().mockResolvedValue(true)
    const card = {
      schemaVersion: 1,
      generationId: 'blank-secret-key-values',
      title: '配置环境变量',
      mode: 'interactive',
      template: 'form',
      cards: [{
        id: 'environment',
        presentation: { variant: 'form', title: '环境变量' },
        form: { sections: [{ id: 'main', fields: [{ id: 'secrets', type: 'key_value', label: '密钥变量', valueMode: 'secret' }] }] },
        actions: [{
          id: 'save',
          type: 'tool',
          label: '保存配置',
          operationId: 'saveConfig',
          bindings: [{ target: '/environment', value: { type: 'field', fieldId: 'secrets' } }],
        }],
      }],
    }
    render(<AIInteractionCards arguments={card} onAction={onAction} />)

    const save = screen.getByRole('button', { name: '保存配置' })
    await waitFor(() => expect(save).toBeEnabled())
    fireEvent.click(save)

    await waitFor(() => expect(onAction).toHaveBeenCalledWith(expect.objectContaining({
      payload: expect.objectContaining({ arguments: {} }),
    })))
  })

  it('does not let an empty row satisfy a required secret key-value field', async () => {
    const card = {
      schemaVersion: 1,
      generationId: 'required-secret-key-values',
      title: '配置环境变量',
      mode: 'interactive',
      template: 'form',
      cards: [{
        id: 'environment',
        presentation: { variant: 'form', title: '环境变量' },
        form: { sections: [{ id: 'main', fields: [{ id: 'secrets', type: 'key_value', label: '密钥变量', required: true, valueMode: 'secret' }] }] },
        actions: [{
          id: 'save',
          type: 'tool',
          label: '保存配置',
          operationId: 'saveConfig',
          bindings: [{ target: '/environment', value: { type: 'field', fieldId: 'secrets' } }],
        }],
      }],
    }
    render(<AIInteractionCards arguments={card} onAction={vi.fn()} />)

    const save = screen.getByRole('button', { name: '保存配置' })
    expect(save).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'Add entry' }))
    expect(save).toBeDisabled()
    fireEvent.change(screen.getByLabelText('Key'), { target: { value: 'API_TOKEN' } })
    expect(save).toBeDisabled()
    fireEvent.change(screen.getByLabelText('Value'), { target: { value: 'user-entered-token' } })
    await waitFor(() => expect(save).toBeEnabled())
  })

  it('keeps a tool action disabled until required fields are valid', async () => {
    const card = structuredClone(candidatesCard) as unknown as {
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
    const card = structuredClone(candidatesCard) as unknown as {
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

  it('renders an Agent-validated card without maintaining a second browser schema', () => {
    render(
      <AIInteractionCards
        arguments={{
          schemaVersion: 1,
          generationId: 'template-selection',
          title: '请选择应用模板',
          mode: 'interactive',
          template: 'candidates',
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

    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.getByText('应用模板市场')).toBeInTheDocument()
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

  it('uses a searchable selector when a card has many candidates', async () => {
    const onAction = vi.fn<(action: AIUIAction) => Promise<boolean>>().mockResolvedValue(true)
    const card = structuredClone(candidatesCard) as unknown as {
      cards: Array<{
        form: { sections: Array<{ fields: Array<Record<string, unknown>> }> }
        actions: unknown[]
      }>
    }
    card.cards[0]!.form.sections[0]!.fields = [{
      id: 'templateId',
      type: 'select',
      label: '应用模板',
      required: true,
      submissionFormat: 'label_value',
      options: [
        { value: 'postgresql', label: 'PostgreSQL' },
        { value: 'redis', label: 'Redis' },
        { value: 'grafana', label: 'Grafana' },
        { value: 'prometheus', label: 'Prometheus' },
        { value: 'minio', label: 'MinIO' },
        { value: 'n8n', label: 'n8n' },
      ],
    }]
    card.cards[0]!.actions = [{
      id: 'continue',
      type: 'send_message',
      label: '继续配置',
      message: '继续配置 {{templateId}}。',
    }]

    render(<AIInteractionCards arguments={card as unknown as Record<string, unknown>} onAction={onAction} />)
    fireEvent.click(screen.getByRole('button', { name: '应用模板 *' }))
    const search = screen.getByPlaceholderText('Search')
    fireEvent.change(search, { target: { value: 'graf' } })
    expect(screen.queryByRole('button', { name: /^PostgreSQL/ })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /Grafana/ }))
    const continueButton = screen.getByRole('button', { name: '继续配置' })
    await waitFor(() => expect(continueButton).toBeEnabled())
    fireEvent.click(continueButton)

    await waitFor(() => expect(onAction).toHaveBeenCalledWith(expect.objectContaining({
      type: 'send_message',
      payload: { message: '继续配置 Grafana (grafana)。' },
    })))
  })

  it('isolates DOM controls when historical cards reuse the same field and option IDs', async () => {
    const card = {
      schemaVersion: 1,
      generationId: 'storage-selection-first',
      title: '数据方案',
      mode: 'interactive',
      template: 'form',
      cards: [{
        id: 'deployment',
        presentation: { variant: 'form', title: '运行配置' },
        form: {
          sections: [{
            id: 'storage',
            fields: [{
              id: 'dataStrategy',
              type: 'select',
              display: 'segmented',
              label: '数据存储方案',
              required: true,
              defaultValue: 'postgres_redis',
              options: [
                { value: 'postgres_redis', label: 'PostgreSQL + Redis' },
                { value: 'sqlite', label: 'SQLite 本地文件' },
              ],
            }],
          }],
        },
        actions: [{
          id: 'continue',
          type: 'send_message',
          label: '提交复现测试',
          message: '选择 {{dataStrategy}}。',
          emphasis: 'primary',
        }],
      }],
    }
    const secondCard = structuredClone(card)
    secondCard.generationId = 'storage-selection-second'
    const { container } = render(
      <div data-testid="timeline">
        <AIInteractionCards arguments={card} onAction={vi.fn()} />
        <AIInteractionCards arguments={secondCard} onAction={vi.fn()} />
      </div>,
    )
    const timeline = screen.getByTestId('timeline')
    timeline.scrollTop = 120
    const ids = [...container.querySelectorAll<HTMLElement>('[id]')].map(element => element.id)
    expect(new Set(ids).size).toBe(ids.length)
    expect(container.querySelector('[id*="dataStrategy"]')).not.toBeInTheDocument()

    const postgresOptions = screen.getAllByRole('radio', { name: 'PostgreSQL + Redis' })
    const sqliteOptions = screen.getAllByRole('radio', { name: 'SQLite 本地文件' })
    expect(postgresOptions).toHaveLength(2)
    expect(sqliteOptions).toHaveLength(2)
    expect(postgresOptions[0]).toBeChecked()
    expect(postgresOptions[1]).toBeChecked()

    fireEvent.click(sqliteOptions[1]!)

    await waitFor(() => expect(sqliteOptions[1]).toBeChecked())
    expect(sqliteOptions[0]).not.toBeChecked()
    expect(postgresOptions[0]).toBeChecked()
    expect(postgresOptions[1]).not.toBeChecked()
    expect(timeline.scrollTop).toBe(120)
  })

  it('renders card descriptions as safe markdown and ignores model HTML', () => {
    const card = structuredClone(candidatesCard) as unknown as {
      description?: string
      cards: Array<{ presentation: { description?: string } }>
    }
    card.description = '请选择 **可信来源** 的数据库。'
    card.cards[0]!.presentation.description = '适合生产环境。<script>window.bad = true</script>'

    const { container } = render(<AIInteractionCards arguments={card as unknown as Record<string, unknown>} onAction={vi.fn()} />)

    expect(screen.getByText('可信来源').tagName).toBe('STRONG')
    expect(container.querySelector('script')).not.toBeInTheDocument()
  })

  it('allows submitting when a required field is hidden by visibleWhen, omits hidden defaults/residuals, and re-enforces validation when revealed', async () => {
    const onAction = vi.fn<(action: AIUIAction) => Promise<boolean>>().mockResolvedValue(true)
    const conditionalCard = {
      schemaVersion: 1,
      generationId: 'visible-when-deadlock-fix',
      title: '配置访问入口',
      mode: 'interactive',
      template: 'form',
      cards: [{
        id: 'gateway',
        presentation: { variant: 'form', title: '网关配置' },
        form: {
          sections: [{
            id: 'main',
            fields: [
              {
                id: 'exposeMode',
                type: 'select',
                display: 'radio',
                label: '暴露模式',
                required: true,
                defaultValue: 'auto',
                options: [
                  { value: 'auto', label: '自动分配域名' },
                  { value: 'custom', label: '自定义域名' },
                ],
              },
              {
                id: 'customDomain',
                type: 'text',
                label: '自定义域名',
                required: true,
                defaultValue: 'hidden-default.example.com',
                visibleWhen: { fieldId: 'exposeMode', operator: 'equals', value: 'custom' },
              },
              {
                id: 'emptyRequiredHidden',
                type: 'text',
                label: '必填隐藏证书名',
                required: true,
                visibleWhen: { fieldId: 'exposeMode', operator: 'equals', value: 'custom' },
              },
            ],
          }],
        },
        actions: [{
          id: 'save',
          type: 'tool',
          label: '保存网关',
          emphasis: 'primary',
          repeatable: true,
          operationId: 'saveGateway',
          bindings: [
            { target: '/exposeMode', value: { type: 'field', fieldId: 'exposeMode' } },
            { target: '/customDomain', value: { type: 'field', fieldId: 'customDomain' } },
            { target: '/certName', value: { type: 'field', fieldId: 'emptyRequiredHidden' } },
          ],
        }],
      }],
    }

    render(<AIInteractionCards arguments={conditionalCard} onAction={onAction} />)

    // 初始 exposeMode=auto：两个 required 隐藏字段不应阻塞提交，且默认值不得混入提交载荷。
    const submitButton = screen.getByRole('button', { name: '保存网关' })
    await waitFor(() => expect(submitButton).toBeEnabled())
    fireEvent.click(submitButton)

    await waitFor(() => expect(onAction).toHaveBeenCalledTimes(1))
    expect(onAction).toHaveBeenLastCalledWith(expect.objectContaining({
      type: 'request_tool',
      payload: {
        operationId: 'saveGateway',
        arguments: { exposeMode: 'auto' },
        message: '保存网关',
      },
    }))

    // 切换为 custom：空的必填字段显现后动态恢复必填拦截。
    fireEvent.click(screen.getByRole('radio', { name: '自定义域名' }))
    await waitFor(() => expect(submitButton).toBeDisabled())

    fireEvent.change(screen.getByLabelText('自定义域名 *'), { target: { value: 'app.example.com' } })
    fireEvent.change(screen.getByLabelText('必填隐藏证书名 *'), { target: { value: 'tls-main' } })
    await waitFor(() => expect(submitButton).toBeEnabled())

    fireEvent.click(submitButton)
    await waitFor(() => expect(onAction).toHaveBeenCalledTimes(2))
    expect(onAction).toHaveBeenLastCalledWith(expect.objectContaining({
      payload: {
        operationId: 'saveGateway',
        arguments: {
          exposeMode: 'custom',
          customDomain: 'app.example.com',
          certName: 'tls-main',
        },
        message: '保存网关',
      },
    }))
  })

  it('unlocks a non-repeatable action button after request failure or downstream tool rejection/failure and surfaces specific errors', async () => {
    const toastErrorSpy = vi.spyOn(toast, 'error').mockImplementation(() => '')
    try {
      const onAction = vi.fn<(action: AIUIAction) => Promise<boolean>>()
        .mockRejectedValueOnce(new ApiError('余额不足，请先充值后重试', { status: 422, code: 'billing.insufficient_balance', path: '/api/v1/ai/conversations/c1/tool-actions' }))
        .mockResolvedValue(true)

      const singleShotCard = {
        schemaVersion: 1,
        generationId: 'retry-card-action',
        title: '创建发布',
        mode: 'interactive',
        template: 'form',
        cards: [{
          id: 'release',
          presentation: { variant: 'form', title: '发布版本' },
          form: {
            sections: [{
              id: 'main',
              fields: [{ id: 'tag', type: 'text', label: '镜像标签', required: true, defaultValue: 'v1.0.0' }],
            }],
          },
          actions: [{
            id: 'deploy',
            type: 'tool',
            label: '执行发布',
            emphasis: 'primary',
            operationId: 'createRelease',
            bindings: [{ target: '/tag', value: { type: 'field', fieldId: 'tag' } }],
          }],
        }],
      }

      const makeRuntime = (blocks: AIAssistantRuntimeValue['timelineProps']['blocks']): AIAssistantRuntimeValue => ({
        timelineProps: { blocks },
      } as unknown as AIAssistantRuntimeValue)

      const { rerender } = render(
        <AIAssistantRuntimeContext value={makeRuntime([])}>
          <AIInteractionCards arguments={singleShotCard} onAction={onAction} />
        </AIAssistantRuntimeContext>,
      )

      const deployButton = screen.getByRole('button', { name: '执行发布' })
      await waitFor(() => expect(deployButton).toBeEnabled())

      // 第一次点击抛错：展示具体后端错误文案且按钮不永久锁死
      fireEvent.click(deployButton)
      await waitFor(() => expect(toastErrorSpy).toHaveBeenCalledWith('余额不足，请先充值后重试'))
      expect(deployButton).toBeEnabled()

      // 第二次点击入队成功：非可重复按钮进入已消费锁定态
      fireEvent.click(deployButton)
      await waitFor(() => expect(onAction).toHaveBeenCalledTimes(2))
      await waitFor(() => expect(deployButton).toBeDisabled())

      // 下游关联工具被用户拒绝（rejected）：按钮自动解锁允许修改参数重试
      rerender(
        <AIAssistantRuntimeContext
          value={makeRuntime([{
            id: 'tool-1',
            turnId: 'turn-2',
            runId: 'run-2',
            index: 1,
            type: 'tool_call',
            toolCallId: 'aitool_1',
            operationId: 'createRelease',
            visibility: 'normal',
            status: 'rejected',
            arguments: { tag: 'v1.0.0' },
            uiActions: [],
          }])}
        >
          <AIInteractionCards arguments={singleShotCard} onAction={onAction} />
        </AIAssistantRuntimeContext>,
      )

      await waitFor(() => expect(deployButton).toBeEnabled())
    }
    finally {
      toastErrorSpy.mockRestore()
    }
  })

  it('keeps optional number and optional minLength text fields valid on mount and renders localized validation errors', async () => {
    const onAction = vi.fn<(action: AIUIAction) => Promise<boolean>>().mockResolvedValue(true)
    const optionalFieldsCard = {
      schemaVersion: 1,
      generationId: 'optional-number-and-minlength',
      title: '可选字段校验',
      mode: 'interactive',
      template: 'form',
      cards: [{
        id: 'config',
        presentation: { variant: 'form', title: '运行参数' },
        form: {
          sections: [{
            id: 'main',
            fields: [
              { id: 'name', type: 'text', label: '应用名称', required: true, defaultValue: 'demo-app' },
              { id: 'replicas', type: 'number', label: '副本数', integer: true, min: 1, max: 10 },
              { id: 'note', type: 'text', label: '备注说明', minLength: 4, maxLength: 20 },
            ],
          }],
        },
        actions: [{
          id: 'save',
          type: 'tool',
          label: '保存参数',
          emphasis: 'primary',
          repeatable: true,
          operationId: 'saveRuntimeParams',
          bindings: [
            { target: '/name', value: { type: 'field', fieldId: 'name' } },
            { target: '/replicas', value: { type: 'field', fieldId: 'replicas' } },
            { target: '/note', value: { type: 'field', fieldId: 'note' } },
          ],
        }],
      }],
    }

    render(<AIInteractionCards arguments={optionalFieldsCard} onAction={onAction} />)

    const saveButton = screen.getByRole('button', { name: '保存参数' })
    await waitFor(() => expect(saveButton).toBeEnabled())

    // 未填写可选 number 与可选 minLength 文本时直接提交，空可选值不混入 arguments
    fireEvent.click(saveButton)
    await waitFor(() => expect(onAction).toHaveBeenCalledTimes(1))
    expect(onAction).toHaveBeenLastCalledWith(expect.objectContaining({
      payload: {
        operationId: 'saveRuntimeParams',
        arguments: { name: 'demo-app' },
        message: '保存参数',
      },
    }))

    // 填入不满足约束的值：按钮禁用，且展示本地化错误而非 Zod 默认英文报错
    const replicasInput = screen.getByLabelText('副本数')
    const noteInput = screen.getByLabelText('备注说明')
    fireEvent.change(replicasInput, { target: { value: '0' } })
    fireEvent.change(noteInput, { target: { value: 'ab' } })

    await waitFor(() => expect(saveButton).toBeDisabled())
    const alerts = screen.getAllByRole('alert').map(element => element.textContent ?? '')
    expect(alerts.some(text => text.includes('1'))).toBe(true)
    expect(alerts.some(text => text.includes('4'))).toBe(true)
    expect(alerts.every(text => !/Too small|Invalid input|Expected number/i.test(text))).toBe(true)

    // 清空回未填状态：恢复合法可提交
    fireEvent.change(replicasInput, { target: { value: '' } })
    fireEvent.change(noteInput, { target: { value: '' } })
    await waitFor(() => expect(saveButton).toBeEnabled())
  })

  it('keeps a consumed single-shot button locked across later failed turns and pagination, while unlocking when its own run is interrupted', async () => {
    const onAction = vi.fn<(action: AIUIAction) => Promise<boolean>>().mockResolvedValue(true)
    const singleShotCard = {
      schemaVersion: 1,
      generationId: 'single-shot-isolation',
      title: '触发发布',
      mode: 'interactive',
      template: 'form',
      cards: [{
        id: 'release',
        presentation: { variant: 'form', title: '发布配置' },
        form: {
          sections: [{
            id: 'main',
            fields: [{ id: 'tag', type: 'text', label: '版本标签', required: true, defaultValue: 'v2.0.0' }],
          }],
        },
        actions: [{
          id: 'deploy',
          type: 'tool',
          label: '确认发布',
          emphasis: 'primary',
          repeatable: false,
          operationId: 'createRelease',
          bindings: [{ target: '/tag', value: { type: 'field', fieldId: 'tag' } }],
        }],
      }],
    }

    const turn1Blocks: AIAssistantRuntimeValue['timelineProps']['blocks'] = [{
      id: 'turn1-msg',
      turnId: 'turn-1',
      runId: 'run-1',
      index: 1_000_000,
      type: 'message',
      role: 'assistant',
      status: 'completed',
      text: '请确认发布参数',
      createdAt: '2026-11-17T10:00:00Z',
    }]

    const makeRuntime = (blocks: AIAssistantRuntimeValue['timelineProps']['blocks']): AIAssistantRuntimeValue => ({
      timelineProps: { blocks },
    } as unknown as AIAssistantRuntimeValue)

    const { rerender, unmount } = render(
      <AIAssistantRuntimeContext value={makeRuntime(turn1Blocks)}>
        <AIInteractionCards arguments={singleShotCard} onAction={onAction} />
      </AIAssistantRuntimeContext>,
    )

    const deployButton = screen.getByRole('button', { name: '确认发布' })
    await waitFor(() => expect(deployButton).toBeEnabled())

    fireEvent.click(deployButton)
    await waitFor(() => expect(onAction).toHaveBeenCalledOnce())
    await waitFor(() => expect(deployButton).toBeDisabled())

    // Turn 2（自身关联的 run-2）成功完成
    const turn2SucceededBlocks: AIAssistantRuntimeValue['timelineProps']['blocks'] = [
      ...turn1Blocks,
      {
        id: 'turn2-tool',
        turnId: 'turn-2',
        runId: 'run-2',
        index: 2_000_000,
        type: 'tool_call',
        toolCallId: 'aitool_2',
        operationId: 'createRelease',
        visibility: 'normal',
        status: 'succeeded',
        arguments: { tag: 'v2.0.0' },
        uiActions: [],
      },
    ]
    rerender(
      <AIAssistantRuntimeContext value={makeRuntime(turn2SucceededBlocks)}>
        <AIInteractionCards arguments={singleShotCard} onAction={onAction} />
      </AIAssistantRuntimeContext>,
    )
    expect(deployButton).toBeDisabled()

    // 向上翻页前置插入 Turn 0 + 后续 Turn 3 发生 failed 与 canceled：绝不能把已成功的 Turn 2 单次按钮误解锁！
    const withOlderAndFailedTurn3: AIAssistantRuntimeValue['timelineProps']['blocks'] = [
      {
        id: 'turn0-status',
        turnId: 'turn-0',
        runId: 'run-0',
        index: 1,
        type: 'run_status',
        status: 'failed',
        errorCode: 'ai.run_failed',
      },
      ...turn2SucceededBlocks,
      {
        id: 'turn3-tool-failed',
        turnId: 'turn-3',
        runId: 'run-3',
        index: 3_000_000,
        type: 'tool_call',
        toolCallId: 'aitool_3',
        operationId: 'createRelease',
        visibility: 'normal',
        status: 'failed',
        arguments: { tag: 'v3.0.0' },
        uiActions: [],
      },
      {
        id: 'turn3-status',
        turnId: 'turn-3',
        runId: 'run-3',
        index: 3_000_001,
        type: 'run_status',
        status: 'canceled',
      },
    ]
    rerender(
      <AIAssistantRuntimeContext value={makeRuntime(withOlderAndFailedTurn3)}>
        <AIInteractionCards arguments={singleShotCard} onAction={onAction} />
      </AIAssistantRuntimeContext>,
    )
    expect(deployButton).toBeDisabled()
    unmount()

    // 单独验证：若自身关联的 Run 终态为 interrupted，按钮正常解锁可重试
    const onInterruptedAction = vi.fn<(action: AIUIAction) => Promise<boolean>>().mockResolvedValue(true)
    const secondRender = render(
      <AIAssistantRuntimeContext value={makeRuntime(turn1Blocks)}>
        <AIInteractionCards arguments={singleShotCard} onAction={onInterruptedAction} />
      </AIAssistantRuntimeContext>,
    )
    const retryableButton = screen.getByRole('button', { name: '确认发布' })
    await waitFor(() => expect(retryableButton).toBeEnabled())
    fireEvent.click(retryableButton)
    await waitFor(() => expect(retryableButton).toBeDisabled())

    secondRender.rerender(
      <AIAssistantRuntimeContext
        value={makeRuntime([
          ...turn1Blocks,
          {
            id: 'turn2-interrupted',
            turnId: 'turn-2',
            runId: 'run-2',
            index: 2_000_000,
            type: 'run_status',
            status: 'interrupted',
          },
        ])}
      >
        <AIInteractionCards arguments={singleShotCard} onAction={onInterruptedAction} />
      </AIAssistantRuntimeContext>,
    )
    await waitFor(() => expect(retryableButton).toBeEnabled())
    expect(secondRender.container.querySelector('[data-ai-card-submitted]')).toBeNull()
  })

  it('collapses completed form cards into a submitted summary without leaking secret values', async () => {
    const onAction = vi.fn<(action: AIUIAction) => Promise<boolean>>().mockResolvedValue(true)
    const formCard = {
      schemaVersion: 1,
      generationId: 'form-collapse-summary',
      title: '凭据接入',
      mode: 'interactive',
      template: 'form',
      cards: [{
        id: 'git-cred',
        presentation: { variant: 'form', title: '接入代码源' },
        form: {
          sections: [{
            id: 'main',
            fields: [
              { id: 'username', type: 'text', label: '用户名', required: true, defaultValue: 'snowykami' },
              { id: 'token', type: 'secret', label: '访问令牌', required: true, generation: 'disabled' },
            ],
          }],
        },
        actions: [{
          id: 'save',
          type: 'tool',
          label: '保存凭据',
          emphasis: 'primary',
          repeatable: false,
          operationId: 'build_save_git_account',
          bindings: [
            { target: '/username', value: { type: 'field', fieldId: 'username' } },
            { target: '/token', value: { type: 'field', fieldId: 'token' } },
          ],
        }],
      }],
    }

    const { container } = render(<AIInteractionCards arguments={formCard} onAction={onAction} />)
    const secretInput = screen.getByLabelText(/访问令牌/)
    fireEvent.change(secretInput, { target: { value: 'super-secret-pat-value' } })
    const saveButton = screen.getByRole('button', { name: '保存凭据' })
    await waitFor(() => expect(saveButton).toBeEnabled())

    fireEvent.click(saveButton)
    await waitFor(() => expect(screen.getByText('Submitted')).toBeInTheDocument())
    expect(container.querySelector('[data-ai-card-submitted="true"]')).not.toBeNull()
    expect(screen.getByText('用户名: snowykami')).toBeInTheDocument()
    expect(container.textContent).not.toContain('super-secret-pat-value')
    expect((secretInput as HTMLInputElement).value).toBe('')
  })

  it('does not falsely claim Submitted on unsubmitted historical form cards or while downstream approval is pending, and re-opens on rejection', async () => {
    const formCard = {
      schemaVersion: 1,
      generationId: 'historical-unsubmitted',
      title: '配置表单',
      mode: 'interactive',
      template: 'form',
      cards: [{
        id: 'cfg',
        presentation: { variant: 'form', title: '填写域名', description: '请填写需要绑定的访问域名' },
        form: {
          sections: [{
            id: 'main',
            fields: [{ id: 'host', type: 'text', label: '域名', required: true, defaultValue: 'app.example.com' }],
          }],
        },
        actions: [{
          id: 'save',
          type: 'tool',
          label: '提交',
          emphasis: 'primary',
          repeatable: false,
          operationId: 'network_create_gateway_route',
          bindings: [{ target: '/host', value: { type: 'field', fieldId: 'host' } }],
        }],
      }],
    }

    const historicalRender = render(<AIInteractionCards arguments={formCard} historical onAction={vi.fn(async () => true)} />)
    expect(historicalRender.container.querySelector('[data-ai-card-submitted]')).toBeNull()
    expect(screen.queryByText('Submitted')).not.toBeInTheDocument()
    expect(screen.getByText('请填写需要绑定的访问域名')).toBeInTheDocument()
    historicalRender.unmount()

    let dispatchedSeq = 0
    const onAction = vi.fn<(action: AIUIAction) => Promise<boolean>>(async () => {
      dispatchedSeq = 1
      return true
    })
    const makeRuntime = (
      runStatuses: Record<string, 'queued' | 'running' | 'waiting_approval' | 'completed' | 'failed' | 'canceled' | 'interrupted'>,
      blocks: AIAssistantRuntimeValue['timelineProps']['blocks'],
    ): AIAssistantRuntimeValue => ({
      runStatuses,
      readLastDispatchedActionRun: () => dispatchedSeq > 0 ? { sequence: dispatchedSeq, runId: 'run-route-1', turnId: 'turn-route-1' } : null,
      timelineProps: { blocks },
    } as unknown as AIAssistantRuntimeValue)

    const { container, rerender } = render(
      <AIAssistantRuntimeContext value={makeRuntime({}, [])}>
        <AIInteractionCards arguments={formCard} onAction={onAction} />
      </AIAssistantRuntimeContext>,
    )

    const submitButton = screen.getByRole('button', { name: '提交' })
    await waitFor(() => expect(submitButton).toBeEnabled())
    fireEvent.click(submitButton)
    await waitFor(() => expect(onAction).toHaveBeenCalledOnce())

    // While waiting_approval: card collapses cleanly, does NOT claim Submitted yet
    rerender(
      <AIAssistantRuntimeContext value={makeRuntime({ 'run-route-1': 'waiting_approval' }, [{
        id: 'route-tool-1',
        turnId: 'turn-route-1',
        runId: 'run-route-1',
        index: 10,
        type: 'tool_call',
        toolCallId: 'tc-route-1',
        operationId: 'network_create_gateway_route',
        visibility: 'normal',
        status: 'awaiting_approval',
        arguments: { host: 'app.example.com' },
        uiActions: [],
      }])}
      >
        <AIInteractionCards arguments={formCard} onAction={onAction} />
      </AIAssistantRuntimeContext>,
    )
    expect(container.querySelector('[data-ai-card-submitted]')).toBeNull()
    expect(screen.queryByText('Submitted')).not.toBeInTheDocument()
    expect(container.querySelector('form')?.className).toContain('hidden')

    // Once user rejects the downstream tool approval: card re-expands and unlocks submit button
    rerender(
      <AIAssistantRuntimeContext value={makeRuntime({ 'run-route-1': 'completed' }, [{
        id: 'route-tool-1',
        turnId: 'turn-route-1',
        runId: 'run-route-1',
        index: 10,
        type: 'tool_call',
        toolCallId: 'tc-route-1',
        operationId: 'network_create_gateway_route',
        visibility: 'normal',
        status: 'rejected',
        arguments: { host: 'app.example.com' },
        uiActions: [],
      }])}
      >
        <AIInteractionCards arguments={formCard} onAction={onAction} />
      </AIAssistantRuntimeContext>,
    )
    expect(container.querySelector('form')?.className).not.toContain('hidden')
    await waitFor(() => expect(screen.getByRole('button', { name: '提交' })).toBeEnabled())
  })
})
