import { render, screen } from '@testing-library/react'
import { beforeAll, describe, expect, it } from 'vitest'
import i18next from '@/i18n'
import { DeploymentReplicaBadge } from './deployment-replica-badge'

describe('deployment replica badge', () => {
  beforeAll(async () => {
    await i18next.changeLanguage('zh-CN')
  })

  it('shows the current and desired replica counts for an observed workload', () => {
    render(<DeploymentReplicaBadge desiredReplicas={3} prefix="生产" readyReplicas={2} status="progressing" />)

    expect(screen.getByText('2/3')).toBeVisible()
    expect(screen.getByText('部署中')).toBeVisible()
    expect(screen.getByText('生产')).toBeVisible()
  })

  it('uses the warning undeployed state when no workload exists', () => {
    render(<DeploymentReplicaBadge deployed={false} desiredReplicas={0} readyReplicas={0} status="not-deployed" />)

    expect(screen.getByText('未部署')).toBeVisible()
    expect(screen.queryByText('0/0')).not.toBeInTheDocument()
  })

  it('shows an observed zero replica workload as scaled down instead of ready', () => {
    render(<DeploymentReplicaBadge availableReplicas={0} desiredReplicas={0} readyReplicas={0} status="ready" />)

    const badge = screen.getByText('已缩容').parentElement
    expect(screen.getByText('0/0')).toBeVisible()
    expect(badge).toHaveClass('text-zinc-700')
    expect(badge).not.toHaveClass('text-success')
  })

  it('downgrades an inconsistent ready status when replicas are not ready and available', () => {
    render(<DeploymentReplicaBadge availableReplicas={0} desiredReplicas={1} readyReplicas={0} status="ready" />)

    const badge = screen.getByText('部署中').parentElement
    expect(screen.getByText('0/1')).toBeVisible()
    expect(badge).toHaveClass('text-warning')
  })

  it('keeps a zero replica rollout progressing until Kubernetes observes the new generation', () => {
    render(<DeploymentReplicaBadge availableReplicas={0} desiredReplicas={0} readyReplicas={0} status="progressing" />)

    expect(screen.getByText('部署中')).toBeVisible()
    expect(screen.getByText('0/0')).toBeVisible()
    expect(screen.queryByText('已缩容')).not.toBeInTheDocument()
  })

  it('does not render zero defaults when the runtime observation is unavailable', () => {
    render(<DeploymentReplicaBadge desiredReplicas={0} readyReplicas={0} status="unavailable" />)

    expect(screen.getByText('不可用')).toBeVisible()
    expect(screen.queryByText('0/0')).not.toBeInTheDocument()
  })
})
