import type { BuildRun } from '@/api'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { BuildCommitDetails } from './build-commit-details'

const run = { sourceCommit: '6e7294a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7', sourceCommitMessage: 'feat: add login\n\nAdds OIDC login.' } as Pick<BuildRun, 'sourceCommit' | 'sourceCommitMessage'>
const commitUrl = 'https://git.example.test/team/app/commit/6e7294a'

function stubHoverCapable(matches: boolean) {
  vi.stubGlobal('matchMedia', (query: string) => ({ matches, media: query, addEventListener: () => undefined, removeEventListener: () => undefined }))
}

describe('build commit details', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('opens the commit message on tap for touch devices and links to the Git platform from the details', async () => {
    stubHoverCapable(false)
    const user = userEvent.setup()
    render(<BuildCommitDetails author="Developer" commitUrl={commitUrl} run={run} />)

    await user.click(screen.getByRole('button', { name: i18next.t('buildsPage.commitDetailsLabel', { commit: '6e7294a' }) }))

    expect(await screen.findByText('feat: add login')).toBeInTheDocument()
    expect(screen.getByText('Adds OIDC login.')).toBeInTheDocument()
    expect(screen.getByText(run.sourceCommit)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: i18next.t('buildsPage.viewCommitOnGit') })).toHaveAttribute('href', commitUrl)
  })

  it('keeps the short SHA as a Git link and shows details on hover for pointer devices', async () => {
    stubHoverCapable(true)
    const user = userEvent.setup()
    render(<BuildCommitDetails author="Developer" commitUrl={commitUrl} run={run} />)

    const link = screen.getByRole('link', { name: '6e7294a' })
    expect(link).toHaveAttribute('href', commitUrl)
    await user.hover(link)

    expect(await screen.findByText('feat: add login')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: i18next.t('buildsPage.viewCommitOnGit') })).not.toBeInTheDocument()
  })

  it('says when no commit message was recorded', async () => {
    stubHoverCapable(false)
    const user = userEvent.setup()
    render(<BuildCommitDetails author="" commitUrl="" run={{ ...run, sourceCommitMessage: '' }} />)

    await user.click(screen.getByRole('button'))
    expect(await screen.findByText(i18next.t('buildsPage.commitMessageUnavailable'))).toBeInTheDocument()
  })
})
