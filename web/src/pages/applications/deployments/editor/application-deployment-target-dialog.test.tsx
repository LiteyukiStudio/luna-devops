import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DeploymentTargetDialogFooter } from './application-deployment-target-dialog'
import '@/i18n'

describe('deployment target dialog footer', () => {
  it('shows a secondary save and primary redeploy action for changed running instances', () => {
    const onSaveAndRedeploy = vi.fn()
    render(
      <DeploymentTargetDialogFooter
        canRedeploy
        hasRuntimeChanges
        saveDisabled={false}
        onSaveAndRedeploy={onSaveAndRedeploy}
      />,
    )

    expect(screen.getByRole('status')).toBeVisible()
    expect(screen.getByRole('button', { name: /仅保存|Save only/ })).toBeEnabled()
    const redeploy = screen.getByRole('button', { name: /保存并重新部署|Save and redeploy/ })
    fireEvent.click(redeploy)
    expect(onSaveAndRedeploy).toHaveBeenCalledOnce()
  })

  it('keeps save-only available when a changed stopped target cannot redeploy', () => {
    render(
      <DeploymentTargetDialogFooter
        canRedeploy={false}
        hasRuntimeChanges
        saveDisabled={false}
        onSaveAndRedeploy={vi.fn()}
      />,
    )

    expect(screen.getByRole('status')).toBeVisible()
    expect(screen.getByRole('button', { name: /仅保存|Save only/ })).toBeEnabled()
    expect(screen.getByRole('button', { name: /保存并重新部署|Save and redeploy/ })).toBeDisabled()
  })
})
