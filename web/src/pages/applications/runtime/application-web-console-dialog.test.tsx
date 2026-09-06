import type { Release } from '@/api'
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ApplicationWebConsoleDialog } from './application-web-console-dialog'

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

describe('application web console CLI command', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
  })

  it('uses stable resource IDs and tracks the selected container', () => {
    render(
      <TooltipProvider>
        <ApplicationWebConsoleDialog projectId="prj_123" release={release} onOpenChange={vi.fn()} />
      </TooltipProvider>,
    )

    const command = screen.getByRole('group', { name: 'CLI command' })
    expect(command).toHaveTextContent(`luna deployment exec server=${window.location.origin} projectId=prj_123 applicationId=app_456 targetId=dplt_789`)

    fireEvent.change(screen.getByRole('textbox', { name: 'Container' }), { target: { value: 'worker' } })
    expect(command).toHaveTextContent(`luna deployment exec server=${window.location.origin} projectId=prj_123 applicationId=app_456 targetId=dplt_789 container=worker`)
  })
})
