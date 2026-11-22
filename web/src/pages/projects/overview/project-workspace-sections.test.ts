import { describe, expect, it } from 'vitest'
import {
  legacyProjectWorkspaceSearch,
  normalizeProjectAIDiagnosisTab,
  normalizeProjectSharedConfigTab,
  normalizeProjectWorkspaceSection,
  projectWorkspaceSectionPath,
  projectWorkspaceSections,
} from './project-workspace-sections'

describe('project workspace sections', () => {
  it('starts with the overview as the default landing section, followed by applications, and includes AI diagnosis in the sidebar', () => {
    expect(projectWorkspaceSections.map(section => section.value)).toEqual(['overview', 'apps', 'shared-config', 'volumes', 'members', 'topology', 'notifications', 'ai-diagnosis'])
    expect(normalizeProjectWorkspaceSection(null)).toBe('overview')
    expect(normalizeProjectWorkspaceSection('unknown')).toBe('overview')
    expect(normalizeProjectWorkspaceSection('notifications')).toBe('notifications')
    expect(normalizeProjectWorkspaceSection('ai-diagnosis')).toBe('ai-diagnosis')
    expect(projectWorkspaceSectionPath('prj 1', 'overview')).toBe('/projects/prj%201')
    expect(projectWorkspaceSectionPath('prj_1', 'apps')).toBe('/projects/prj_1?tab=apps')
    expect(projectWorkspaceSectionPath('prj_1', 'notifications')).toBe('/projects/prj_1?tab=notifications')
    expect(projectWorkspaceSectionPath('prj_1', 'ai-diagnosis')).toBe('/projects/prj_1?tab=ai-diagnosis')
  })

  it('maps legacy shared-config and notification AI automation URLs to their dedicated sections', () => {
    expect(normalizeProjectWorkspaceSection('hooks')).toBe('shared-config')
    expect(legacyProjectWorkspaceSearch(new URLSearchParams('tab=hooks&page=2'))).toBe('?tab=shared-config&page=2&config=hooks')
    expect(legacyProjectWorkspaceSearch(new URLSearchParams('tab=runtime-configs'))).toBe('?tab=shared-config&config=runtime-configs')
    expect(legacyProjectWorkspaceSearch(new URLSearchParams('tab=build-variables'))).toBe('?tab=shared-config')
    expect(legacyProjectWorkspaceSearch(new URLSearchParams('tab=notifications&subtab=ai-automation'))).toBe('?tab=ai-diagnosis')
    expect(legacyProjectWorkspaceSearch(new URLSearchParams('tab=shared-config&config=hooks'))).toBeNull()
    expect(legacyProjectWorkspaceSearch(new URLSearchParams('tab=apps'))).toBeNull()
  })

  it('falls back to build variables for unknown shared config tabs and history for AI diagnosis tabs', () => {
    expect(normalizeProjectSharedConfigTab(null)).toBe('build-variables')
    expect(normalizeProjectSharedConfigTab('volumes')).toBe('build-variables')
    expect(normalizeProjectSharedConfigTab('hooks')).toBe('hooks')
    expect(normalizeProjectAIDiagnosisTab(null)).toBe('history')
    expect(normalizeProjectAIDiagnosisTab('unknown')).toBe('history')
    expect(normalizeProjectAIDiagnosisTab('settings')).toBe('settings')
  })
})
