import type { BuildJob, BuildRun } from '@/api'
import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { describe, expect, it } from 'vitest'
import { BuildRunStatusCell } from './application-build-run-row'

const run = { id: 'bld_1', status: 'running', failureCode: '' } as BuildRun

function job(overrides: Partial<BuildJob>) {
  return { id: 'job_1', buildRunId: 'bld_1', status: 'running', message: 'run_command', stepsCompleted: 4, stepsTotal: 8, failureCode: '', ...overrides } as BuildJob
}

describe('build run status cell', () => {
  it('shows the current phase with completed and known total Dockerfile steps while running', () => {
    render(<BuildRunStatusCell latestJob={job({})} run={run} />)

    const status = screen.getByText(i18next.t('buildsPage.progress.run_command')).closest('[data-slot="status-indicator"]')
    expect(status).toHaveTextContent(i18next.t('buildsPage.stepProgress', { completed: 4, total: 8 }))
  })

  it('omits step progress before any step is known or after the job ends', () => {
    const { rerender } = render(<BuildRunStatusCell latestJob={job({ stepsCompleted: 0, stepsTotal: 0 })} run={run} />)
    expect(screen.queryByText(/\d+\/\d+/)).not.toBeInTheDocument()

    rerender(<BuildRunStatusCell latestJob={job({ status: 'succeeded', message: '' })} run={{ ...run, status: 'succeeded' }} />)
    expect(screen.queryByText(/\d+\/\d+/)).not.toBeInTheDocument()
  })
})
