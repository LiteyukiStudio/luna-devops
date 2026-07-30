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

function includesString<T extends string>(values: readonly T[], value: string): value is T {
  return (values as readonly string[]).includes(value)
}

export function isPlatformRole(role: string | null | undefined): role is PlatformRoleValue {
  return Boolean(role && includesString(PLATFORM_ROLES, role))
}

export function isPlatformAdmin(role: string | null | undefined): boolean {
  return role === PlatformRole.Admin
}

export function isProjectRole(role: string | null | undefined): role is ProjectRoleValue {
  return Boolean(role && includesString(PROJECT_ROLES, role))
}
