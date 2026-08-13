import { describe, expect, it, vi } from 'vitest'
import { openDeploymentTargetDataExport } from './deployment-target-data-export'

describe('openDeploymentTargetDataExport', () => {
  it('waits for authorization before starting the download without opening a blank page', async () => {
    let resolveAuthorization!: () => void
    const authorization = new Promise<{ ticket: string, expiresAt: string }>((resolve) => {
      resolveAuthorization = () => resolve({ ticket: 'ticket with spaces', expiresAt: '2026-07-12T00:01:00Z' })
    })
    const authorize = vi.fn(() => authorization)
    const startDownload = vi.fn()

    const exporting = openDeploymentTargetDataExport('project one', 'app/two', 'target three', { authorize, startDownload })

    expect(authorize).toHaveBeenCalledWith('project one', 'app/two', 'target three')
    expect(startDownload).not.toHaveBeenCalled()

    resolveAuthorization()
    await exporting

    expect(startDownload).toHaveBeenCalledWith('/api/v1/projects/project%20one/applications/app%2Ftwo/deployment-targets/target%20three/data-export?ticket=ticket+with+spaces')
  })

  it('does not start a download when MFA authorization is cancelled or fails', async () => {
    const error = new Error('mfa_challenge_cancelled')
    const authorize = vi.fn(async () => Promise.reject(error))
    const startDownload = vi.fn()

    await expect(openDeploymentTargetDataExport('project', 'app', 'target', {
      authorize,
      startDownload,
    })).rejects.toBe(error)

    expect(startDownload).not.toHaveBeenCalled()
  })
})
