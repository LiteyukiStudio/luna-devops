import { describe, expect, it } from 'vitest'
import {
  legacyProjectWorkspaceSearch,
  normalizeProjectSharedConfigTab,
  normalizeProjectWorkspaceSection,
  projectWorkspaceSectionPath,
  projectWorkspaceSections,
} from './project-workspace-sections'

describe('project workspace sections', () => {
  it('starts with the overview as the default landing section, followed by applications', () => {
    expect(projectWorkspaceSections.map(section => section.value)).toEqual(['overview', 'apps', 'shared-config', 'volumes', 'members', 'topology'])
    expect(normalizeProjectWorkspaceSection(null)).toBe('overview')
    expect(normalizeProjectWorkspaceSection('unknown')).toBe('overview')
    expect(projectWorkspaceSectionPath('prj 1', 'overview')).toBe('/projects/prj%201')
    expect(projectWorkspaceSectionPath('prj_1', 'apps')).toBe('/projects/prj_1?tab=apps')
  })

  it('maps the former build variable, runtime config and hook sections to the shared config tabs', () => {
    expect(normalizeProjectWorkspaceSection('hooks')).toBe('shared-config')
    expect(legacyProjectWorkspaceSearch(new URLSearchParams('tab=hooks&page=2'))).toBe('?tab=shared-config&page=2&config=hooks')
    expect(legacyProjectWorkspaceSearch(new URLSearchParams('tab=runtime-configs'))).toBe('?tab=shared-config&config=runtime-configs')
    expect(legacyProjectWorkspaceSearch(new URLSearchParams('tab=build-variables'))).toBe('?tab=shared-config')
    expect(legacyProjectWorkspaceSearch(new URLSearchParams('tab=shared-config&config=hooks'))).toBeNull()
    expect(legacyProjectWorkspaceSearch(new URLSearchParams('tab=apps'))).toBeNull()
  })

  it('falls back to build variables for unknown shared config tabs', () => {
    expect(normalizeProjectSharedConfigTab(null)).toBe('build-variables')
    expect(normalizeProjectSharedConfigTab('volumes')).toBe('build-variables')
    expect(normalizeProjectSharedConfigTab('hooks')).toBe('hooks')
  })
})
