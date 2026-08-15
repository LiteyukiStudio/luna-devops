import { render, screen } from '@testing-library/react'
import { beforeAll, describe, expect, it } from 'vitest'
import i18next from '@/i18n'
import { DeploymentReplicaBadge } from './deployment-replica-badge'

describe('deployment replica badge', () => {
  beforeAll(async () => {
    await i18next.changeLanguage('zh-CN')
  })

  it('shows the current and desired replica counts for an observed workload', () => {
    render(<DeploymentReplicaBadge desiredReplicas={3} readyReplicas={2} status="progressing" />)

    expect(screen.getByText('2/3')).toBeVisible()
    expect(screen.getByText('部署中')).toBeVisible()
  })

  it('uses the warning undeployed state when no workload exists', () => {
    render(<DeploymentReplicaBadge deployed={false} desiredReplicas={0} readyReplicas={0} status="not-deployed" />)

    expect(screen.getByText('未部署')).toBeVisible()
    expect(screen.queryByText('0/0')).not.toBeInTheDocument()
  })
})
