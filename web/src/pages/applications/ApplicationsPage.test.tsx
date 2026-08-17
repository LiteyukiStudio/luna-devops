import type { Application } from '@/api'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeAll, describe, expect, it } from 'vitest'
import i18next from '@/i18n'
import { ApplicationSummary } from './application-summary'

const application: Application = {
  id: 'app-1',
  projectId: 'project-1',
  identifier: 'example',
  name: '示例应用',
  icon: 'box',
  deleteStatus: 'active',
  deleteMessage: '',
  createdAt: '2026-08-18T00:00:00Z',
  updatedAt: '2026-08-18T00:00:00Z',
}

describe('application summary', () => {
  beforeAll(async () => {
    await i18next.changeLanguage('zh-CN')
  })

  it('shows runtime status separately for each deployment stage', () => {
    render(
      <MemoryRouter>
        <ApplicationSummary
          application={{
            ...application,
            deploymentSummary: {
              targetCount: 2,
              desiredReplicas: 3,
              readyReplicas: 2,
              status: 'degraded',
              targets: [
                { id: 'prod', stage: 'prod', desiredReplicas: 2, readyReplicas: 1, status: 'degraded' },
                { id: 'dev', stage: 'dev', desiredReplicas: 1, readyReplicas: 1, status: 'ready' },
              ],
            },
          }}
          projectId="project-1"
        />
      </MemoryRouter>,
    )

    expect(screen.getByText('生产')).toBeVisible()
    expect(screen.getByText('异常')).toBeVisible()
    expect(screen.getByText('1/2')).toBeVisible()
    expect(screen.getByText('开发')).toBeVisible()
    expect(screen.getByText('就绪')).toBeVisible()
    expect(screen.getByText('1/1')).toBeVisible()
    expect(screen.queryByText('2/3')).not.toBeInTheDocument()
  })

  it('keeps the undeployed state when the application has no deployment target', () => {
    render(
      <MemoryRouter>
        <ApplicationSummary
          application={{
            ...application,
            deploymentSummary: {
              targetCount: 0,
              desiredReplicas: 0,
              readyReplicas: 0,
              status: 'not-deployed',
              targets: [],
            },
          }}
          projectId="project-1"
        />
      </MemoryRouter>,
    )

    expect(screen.getByText('未部署')).toBeVisible()
    expect(screen.queryByText('0/0')).not.toBeInTheDocument()
  })
})
