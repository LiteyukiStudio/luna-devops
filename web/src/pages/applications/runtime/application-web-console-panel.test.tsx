import type { Release } from '@/api'
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ApplicationWebConsolePanel } from './application-web-console-panel'

vi.mock('@/components/common/runtime-terminal-panel', () => ({
  RuntimeTerminalPanel: () => <div>Terminal</div>,
}))

const release: Release = {
  id: 'rel_123',
  projectId: 'prj_123',
  applicationId: 'app_456',
  deploymentTargetId: 'dplt_789',
  buildRunId: 'build_123',
  imageRef: 'registry.example.com/app:latest',
  forceImagePull: false,
  type: 'deploy',
  status: 'succeeded',
  revision: 1,
  rollbackFromId: '',
  message: '',
  createdBy: 'usr_123',
  createdAt: '2026-09-06T00:00:00Z',
}

describe('application web console panel', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
  })

  it('tracks the selected container in the CLI command and toggles fullscreen', async () => {
    render(
      <TooltipProvider>
        <ApplicationWebConsolePanel projectId="prj_123" release={release} onClose={vi.fn()} />
      </TooltipProvider>,
    )

    expect(await screen.findByText('Terminal')).toBeInTheDocument()

    const command = screen.getByRole('group', { name: 'CLI command' })
    expect(command).toHaveTextContent(`luna deployment exec server=${window.location.origin} projectId=prj_123 applicationId=app_456 targetId=dplt_789`)

    fireEvent.change(screen.getByRole('textbox', { name: 'Container' }), { target: { value: 'worker' } })
    expect(command).toHaveTextContent(`luna deployment exec server=${window.location.origin} projectId=prj_123 applicationId=app_456 targetId=dplt_789 container=worker`)

    fireEvent.click(screen.getByRole('button', { name: 'Fullscreen' }))
    expect(screen.getByRole('button', { name: 'Exit fullscreen' })).toBeInTheDocument()
  })
})
