import type { BuildJob, BuildRun } from '@/api'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { describe, expect, it } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { BuildRunStatusCell } from './application-build-run-row'

const run = { id: 'bld_1', status: 'running', failureCode: '' } as BuildRun

function job(overrides: Partial<BuildJob>) {
  return { id: 'job_1', buildRunId: 'bld_1', status: 'running', message: 'run_command', stepsCompleted: 5, stepsTotal: 7, currentStep: '[runner 5/7] RUN pnpm build', failureCode: '', ...overrides } as BuildJob
}

function renderCell(latestJob: BuildJob, buildRun: BuildRun = run) {
  return render(<TooltipProvider><BuildRunStatusCell latestJob={latestJob} run={buildRun} /></TooltipProvider>)
}

describe('build run status cell', () => {
  it('shows building with completed and known total steps, and the current phase on hover', async () => {
    const user = userEvent.setup()
    renderCell(job({}))

    const status = screen.getByText(i18next.t('buildsPage.building')).closest('[data-slot="status-indicator"]')
    expect(status).toHaveTextContent('5/7')
    expect(screen.queryByText(i18next.t('buildsPage.progress.run_command'))).not.toBeInTheDocument()

    await user.hover(status!)
    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip).toHaveTextContent(i18next.t('buildsPage.progress.run_command'))
    expect(tooltip).toHaveTextContent('[runner 5/7] RUN pnpm build')
    expect(tooltip).toHaveTextContent(i18next.t('buildsPage.stepProgress', { completed: 5, total: 7 }))
  })

  it('truncates long instructions in the hover hint with an ellipsis', async () => {
    const user = userEvent.setup()
    const instruction = `[builder 2/3] RUN ${'x'.repeat(300)}`
    renderCell(job({ currentStep: instruction }))

    await user.hover(screen.getByText(i18next.t('buildsPage.building')))
    const tooltip = await screen.findByRole('tooltip')
    expect(tooltip.textContent).toContain('…')
    expect(tooltip.textContent).not.toContain(instruction)
  })

  it('omits step counts before any step is known and after the job ends', () => {
    const { rerender } = renderCell(job({ stepsCompleted: 0, stepsTotal: 0 }))
    expect(screen.getByText(i18next.t('buildsPage.building'))).toBeInTheDocument()
    expect(screen.queryByText(/\d+\/\d+/)).not.toBeInTheDocument()

    rerender(<TooltipProvider><BuildRunStatusCell latestJob={job({ status: 'succeeded', message: '' })} run={{ ...run, status: 'succeeded' }} /></TooltipProvider>)
    expect(screen.getByText(i18next.t('buildsPage.statuses.succeeded'))).toBeInTheDocument()
    expect(screen.queryByText(/\d+\/\d+/)).not.toBeInTheDocument()
  })
})
