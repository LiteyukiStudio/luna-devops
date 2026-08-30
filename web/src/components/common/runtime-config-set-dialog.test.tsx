import type { ProjectRuntimeConfigSet, ProjectRuntimeConfigSetPayload } from '@/api'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useForm } from 'react-hook-form'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { RuntimeConfigSetDialog } from './runtime-config-set-dialog'

vi.mock('@/components/common/key-value-text-editor', () => ({
  KeyValueTextEditor: ({ initialValue, onChange }: {
    initialValue: Record<string, string>
    onChange: (value: Record<string, string>) => void
  }) => (
    <button type="button" onClick={() => onChange({ UPDATED: 'true' })}>
      {`Update plain config ${initialValue.LOG_LEVEL ?? 'empty'}`}
    </button>
  ),
}))

vi.mock('@/components/common/runtime-config-files-editor', () => ({
  RuntimeConfigFilesEditor: ({ initialValue, onChange }: {
    initialValue: string
    onChange: (value: string) => void
  }) => (
    <button type="button" onClick={() => onChange(`${initialValue}:updated`)}>
      {`Update runtime files ${initialValue || 'empty'}`}
    </button>
  ),
}))

vi.mock('@/components/common/runtime-config-set-secrets-editor', () => ({
  RuntimeConfigSetSecretsEditor: ({ projectId, set }: { projectId: string, set: ProjectRuntimeConfigSet | null }) => (
    <div>{`Secret variables ${projectId}/${set?.id ?? 'new'}`}</div>
  ),
}))

const defaultValues: ProjectRuntimeConfigSetPayload = {
  configFiles: 'config-original',
  enabled: true,
  environmentVariables: [{ key: 'LOG_LEVEL', value: 'info', valueMode: 'public' }],
  name: 'Existing config',
  secretFiles: 'secret-original',
}

interface DialogHarnessProps {
  configFilesValid?: boolean
  pending?: boolean
  secretFilesValid?: boolean
  onSubmit?: (values: ProjectRuntimeConfigSetPayload) => void
}

function DialogHarness({ configFilesValid = true, pending = false, secretFilesValid = true, onSubmit = () => {} }: DialogHarnessProps) {
  const form = useForm<ProjectRuntimeConfigSetPayload>({ defaultValues, mode: 'onChange' })

  return (
    <TooltipProvider>
      <RuntimeConfigSetDialog
        canManageSecrets
        configFilesValid={configFilesValid}
        editingSet={null}
        form={form}
        open
        pending={pending}
        projectId="project-1"
        secretFilesValid={secretFilesValid}
        onConfigFilesValidityChange={() => {}}
        onOpenChange={() => {}}
        onSecretFilesValidityChange={() => {}}
        onSubmit={onSubmit}
      />
    </TooltipProvider>
  )
}

describe('runtime config set dialog', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
  })

  it('renders the shared fields and submits values from every editor', async () => {
    const onSubmit = vi.fn()
    render(<DialogHarness onSubmit={onSubmit} />)

    expect(screen.getByRole('dialog', { name: 'Add config' })).toHaveAccessibleDescription(/attached to deploy configs/i)
    expect(screen.getByText('Secret variables project-1/new')).toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: 'Enabled' })).toHaveAttribute('aria-checked', 'true')

    fireEvent.change(screen.getByRole('textbox', { name: 'Name' }), { target: { value: 'Updated config' } })
    fireEvent.click(screen.getByRole('button', { name: 'Update plain config info' }))
    fireEvent.click(screen.getByRole('button', { name: 'Update runtime files config-original' }))
    fireEvent.click(screen.getByRole('button', { name: 'Update runtime files secret-original' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'Enabled' }))
    const saveButton = screen.getByRole('button', { name: 'Save' })
    await waitFor(() => expect(saveButton).toBeEnabled())
    fireEvent.click(saveButton)

    await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce())
    expect(onSubmit.mock.calls[0][0]).toEqual({
      configFiles: 'config-original:updated',
      enabled: false,
      environmentVariables: [{ key: 'UPDATED', value: 'true', valueMode: 'public' }],
      name: 'Updated config',
      secretFiles: 'secret-original:updated',
    })
  })

  it('disables submission for an invalid name, invalid editors, or a pending request', async () => {
    const { rerender } = render(<DialogHarness />)
    const nameInput = screen.getByRole('textbox', { name: 'Name' })
    const saveButton = screen.getByRole('button', { name: 'Save' })
    await waitFor(() => expect(saveButton).toBeEnabled())

    fireEvent.change(nameInput, { target: { value: '' } })
    await waitFor(() => expect(saveButton).toBeDisabled())
    expect(nameInput).toHaveAttribute('aria-invalid', 'true')
    expect(screen.getByText('This field is required')).toBeInTheDocument()

    fireEvent.change(nameInput, { target: { value: 'Valid config' } })
    await waitFor(() => expect(saveButton).toBeEnabled())

    rerender(<DialogHarness configFilesValid={false} />)
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()

    rerender(<DialogHarness secretFilesValid={false} />)
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()

    rerender(<DialogHarness pending />)
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Save' }).closest('form')).toHaveAttribute('aria-busy', 'true')

    rerender(<DialogHarness />)
    await waitFor(() => expect(screen.getByRole('button', { name: 'Save' })).toBeEnabled())
  })
})
