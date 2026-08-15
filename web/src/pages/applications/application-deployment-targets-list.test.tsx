import { render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ApplicationDeploymentTargetsList } from './application-deployment-targets-list'
import '@/i18n'

describe('application deployment targets layout', () => {
  it('contains intrinsic table width and switches views by content width', () => {
    const { container } = render(
      <ApplicationDeploymentTargetsList
        applicationId="app_1"
        createReleasePending={false}
        deletePending={false}
        deployableBuildRuns={[]}
        items={[]}
        projectId="prj_1"
        pullLatestPending={false}
        restartPending={false}
        rollbackPending={false}
        onCopy={vi.fn()}
        onDeleteTarget={vi.fn()}
        onOpenConsole={vi.fn()}
        onOpenReleaseDialog={vi.fn()}
        onOpenTargetDialog={vi.fn()}
        onPullLatestImageDeploy={vi.fn()}
        onRestart={vi.fn()}
        onRollback={vi.fn()}
        onViewLogs={vi.fn()}
      />,
    )

    expect(container.querySelector('[data-slot="deployment-targets-list"]')).toHaveClass(
      'w-full',
      'min-w-0',
      'max-w-full',
      '@container/deployment-targets',
    )
    expect(container.querySelector('[data-slot="deployment-targets-table"]')).toHaveClass(
      'hidden',
      'min-w-0',
      'max-w-full',
      '@[68rem]/deployment-targets:block',
    )
    expect(container.querySelector('[data-slot="deployment-targets-cards"]')).toHaveClass(
      'min-w-0',
      'max-w-full',
      '@[68rem]/deployment-targets:hidden',
    )
  })
})
