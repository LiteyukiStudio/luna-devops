import type { AIUIAction } from '@/api'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AIInteractionCards } from './interaction-cards'

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
})
