export const PlatformRole = {
  Admin: 'platform_admin',
  User: 'user',
} as const

export type PlatformRoleValue = typeof PlatformRole[keyof typeof PlatformRole]

export const PLATFORM_ROLES = [
  PlatformRole.Admin,
  PlatformRole.User,
] as const

export const ProjectRole = {
  Owner: 'owner',
  Admin: 'admin',
  Developer: 'developer',
  Viewer: 'viewer',
} as const

export type ProjectRoleValue = typeof ProjectRole[keyof typeof ProjectRole]

export const PROJECT_ROLES = [
  ProjectRole.Owner,
  ProjectRole.Admin,
  ProjectRole.Developer,
  ProjectRole.Viewer,
] as const

export function isPlatformAdmin(role: string | null | undefined): boolean {
  return role === PlatformRole.Admin
}

/**
 * 前端消费的项目空间 Action 名，取值与 internal/authz 策略键一致。
 * 是否放行只看 getProject 返回的 currentUserActions，不在前端按角色推导。
 */
export const ProjectAction = {
  ProjectWrite: 'project:write',
  ProjectManage: 'project:manage',
  ProjectOwnerOnly: 'project:owner_only',
  DeploymentUpdate: 'deployment:update',
  VolumeWrite: 'volume:write',
  VolumeImport: 'volume:import',
  VolumeExport: 'volume:export',
  VolumeDelete: 'volume:delete',
} as const

export type ProjectActionValue = typeof ProjectAction[keyof typeof ProjectAction]

export function projectActionAllowed(actions: readonly string[] | null | undefined, action: ProjectActionValue): boolean {
  return actions?.includes(action) ?? false
}
