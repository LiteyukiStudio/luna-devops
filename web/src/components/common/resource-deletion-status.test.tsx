import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { ResourceDeletionStatus } from './resource-deletion-status'

function renderStatus(props: Parameters<typeof ResourceDeletionStatus>[0]) {
  return render(
    <TooltipProvider>
      <ResourceDeletionStatus {...props} />
    </TooltipProvider>,
  )
}

describe('resource deletion status', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
  })

  it('stays hidden for an active resource', () => {
    const { container } = renderStatus({ message: 'Not shown', status: 'active' })

    expect(container).toBeEmptyDOMElement()
  })

  it('shows deletion progress without an irrelevant message', () => {
    renderStatus({ message: 'Ignored while deleting', status: 'deleting' })

    expect(screen.getByText('Deleting')).toBeInTheDocument()
    expect(screen.queryByText('Ignored while deleting')).not.toBeInTheDocument()
  })

  it('shows a trimmed failure message for delete_failed', () => {
    renderStatus({ message: '  Persistent volume cleanup failed  ', status: 'delete_failed' })

    expect(screen.getByText('Delete failed')).toBeInTheDocument()
    expect(screen.getByText('Persistent volume cleanup failed')).toBeInTheDocument()
  })
})
