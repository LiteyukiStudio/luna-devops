import type { ClusterResource, CurrentUser } from '@/api'
import { isPlatformAdmin } from '@/lib/roles'

export function canDeleteClusterResource(user: CurrentUser | undefined, item: ClusterResource) {
  return isPlatformAdmin(user?.role) || Boolean(item.projectId?.trim())
}
