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
        hasRunningInstances
        hasRuntimeChanges
        saveDisabled={false}
        onSaveAndRedeploy={onSaveAndRedeploy}
      />,
    )

    expect(screen.getByRole('status')).toHaveClass('text-warning')
    expect(screen.getByRole('button', { name: /仅保存|Save only/ })).toHaveClass('border')
    const redeploy = screen.getByRole('button', { name: /保存并重新部署|Save and redeploy/ })
    expect(redeploy).toHaveClass('bg-primary')
    fireEvent.click(redeploy)
    expect(onSaveAndRedeploy).toHaveBeenCalledOnce()
  })

  it('keeps a single save action when there is no running instance', () => {
    render(
      <DeploymentTargetDialogFooter
        canRedeploy={false}
        hasRunningInstances={false}
        hasRuntimeChanges
        saveDisabled={false}
        onSaveAndRedeploy={vi.fn()}
      />,
    )

    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /仅保存|Save only/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /保存并重新部署|Save and redeploy/ })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^保存$|^Save$/ })).toHaveClass('bg-primary')
  })
})
