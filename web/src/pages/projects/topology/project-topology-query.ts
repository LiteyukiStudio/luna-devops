import type { ProjectTopologyOrigin } from '@/api'

export const projectTopologyKeys = {
  all: (projectId: string) => ['project-topology', projectId] as const,
  graph: (projectId: string, stage: string, origins: ProjectTopologyOrigin[]) =>
    ['project-topology', projectId, 'graph', stage, origins.join(',')] as const,
  serviceBindings: (projectId: string) => ['project-topology', projectId, 'service-bindings'] as const,
  manualEdges: (projectId: string) => ['project-topology', projectId, 'manual-edges'] as const,
}

export const projectTopologyPageParams = {
  page: 1,
  pageSize: 100,
  sortBy: 'updatedAt',
  sortOrder: 'desc' as const,
}
