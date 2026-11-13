import { describe, expect, it } from 'vitest'
import { buildRunPath, platformEventDetailPath, platformEventResourcePath, projectEventsPath } from './resource-routes'

function event(resourceType: string, overrides: Partial<{ applicationId: string, projectId: string, resourceId: string }> = {}) {
  return { applicationId: 'app_1', projectId: 'prj_1', resourceId: 'res_1', resourceType, ...overrides }
}

describe('resource routes', () => {
  it('focuses a build run inside its application builds tab', () => {
    expect(buildRunPath({ applicationId: 'app_1', id: 'bld_1', projectId: 'prj_1' })).toBe('/projects/prj_1/apps/app_1?tab=builds#buildRunId=bld_1')
  })

  it.each([
    ['build', '/projects/prj_1/apps/app_1?tab=builds#buildRunId=res_1'],
    ['repository_binding', '/projects/prj_1/apps/app_1?tab=builds'],
    ['deployment_target', '/projects/prj_1/apps/app_1?tab=deployments#targetId=res_1'],
    ['release', '/projects/prj_1/apps/app_1?tab=deployments#releaseId=res_1'],
    ['hook', '/projects/prj_1/apps/app_1?tab=deployments'],
    ['gateway', '/projects/prj_1/apps/app_1?tab=gateway'],
    ['certificate', '/projects/prj_1/apps/app_1?tab=gateway'],
    ['service_binding', '/projects/prj_1?tab=topology'],
    ['unknown', '/projects/prj_1/apps/app_1'],
  ])('routes %s events to their resource page', (resourceType, path) => {
    expect(platformEventResourcePath(event(resourceType))).toBe(path)
  })

  it('returns null when the event has no navigable resource', () => {
    expect(platformEventResourcePath(event('project', { applicationId: '' }))).toBeNull()
    expect(platformEventResourcePath(event('build', { projectId: '' }))).toBeNull()
  })

  it('builds event center links', () => {
    expect(platformEventDetailPath({ id: 'evt_1', projectId: 'prj_1' })).toBe('/events?eventId=evt_1&projectId=prj_1')
    expect(projectEventsPath('prj_1', 'build')).toBe('/events?projectId=prj_1&category=build')
  })
})
