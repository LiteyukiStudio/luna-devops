import type { PaginatedResponse, PaginationParams } from './types'

export type ProjectTopologyOrigin = 'service_binding' | 'manual'
export type ProjectTopologyRelationType = 'depends_on' | 'calls' | 'reads_writes' | 'publishes_to' | 'consumes_from'
export type ProjectTopologyStatus = 'ready' | 'unavailable' | 'invalid' | 'disabled' | 'declared' | string

export interface ProjectTopologyDeploymentTarget {
  id: string
  name: string
  stage: string
  clusterId: string
  enabled: boolean
}

export interface ProjectTopologyNode {
  id: string
  kind: 'application' | string
  name: string
  identifier?: string
  status: string
  deploymentTargets: ProjectTopologyDeploymentTarget[]
}

export interface ProjectTopologyEdge {
  id: string
  source: string
  target: string
  sourceDeploymentTargetId?: string
  targetDeploymentTargetId?: string
  origin: ProjectTopologyOrigin
  relationType: ProjectTopologyRelationType
  status: ProjectTopologyStatus
  protocol?: string
  port?: number
  description?: string
  injectionMode?: ServiceBindingInjectionMode
  urlEnvVar?: string
  hostEnvVar?: string
  portEnvVar?: string
}

export interface ProjectTopologyWarning {
  code: string
  edgeId?: string
  nodeId?: string
  detail?: string
}

export interface ProjectTopology {
  generatedAt: string
  nodes: ProjectTopologyNode[]
  edges: ProjectTopologyEdge[]
  warnings: ProjectTopologyWarning[]
}

export interface ProjectTopologyQuery {
  applicationId?: string
  origins?: ProjectTopologyOrigin[]
  stage?: string
}

export interface ProjectTopologyListParams extends PaginationParams {
  enabled?: boolean
}

export type ServiceBindingProtocol = 'http' | 'https' | 'tcp'
export type ServiceBindingInjectionMode = 'url' | 'host_port'

export interface ServiceBinding {
  id: string
  projectId: string
  sourceApplicationId: string
  sourceDeploymentTargetId: string
  targetApplicationId: string
  targetDeploymentTargetId: string
  targetPortName: string
  targetPort: number
  protocol: ServiceBindingProtocol
  path: string
  injectionMode: ServiceBindingInjectionMode
  urlEnvVar: string
  hostEnvVar: string
  portEnvVar: string
  enabled: boolean
  status?: ProjectTopologyStatus
  createdBy: string
  createdAt: string
  updatedAt: string
}

export type ServiceBindingPayload = Pick<ServiceBinding, | 'sourceApplicationId'
  | 'sourceDeploymentTargetId'
  | 'targetApplicationId'
  | 'targetDeploymentTargetId'
  | 'targetPortName'
  | 'protocol'
  | 'path'
  | 'injectionMode'
  | 'urlEnvVar'
  | 'hostEnvVar'
  | 'portEnvVar'
  | 'enabled'>

export interface ServiceBindingMutationResult {
  item?: ServiceBinding
  requiresRedeploy: boolean
  affectedDeploymentTargets: Array<{
    applicationId: string
    deploymentTargetId: string
  }>
}

export interface ProjectTopologyManualEdge {
  id: string
  projectId: string
  sourceApplicationId: string
  sourceDeploymentTargetId?: string | null
  targetApplicationId: string
  targetDeploymentTargetId?: string | null
  relationType: ProjectTopologyRelationType
  protocol: string
  port?: number | null
  description: string
  createdBy: string
  createdAt: string
  updatedAt: string
}

export type ProjectTopologyManualEdgePayload = Pick<ProjectTopologyManualEdge, | 'sourceApplicationId'
  | 'sourceDeploymentTargetId'
  | 'targetApplicationId'
  | 'targetDeploymentTargetId'
  | 'relationType'
  | 'protocol'
  | 'port'
  | 'description'>

export interface ServiceBindingCheckItem {
  code: string
  status: 'passed' | 'warning' | 'failed' | 'unavailable'
  resource?: string
  detail?: string
}

export interface ServiceBindingCheckResult {
  bindingId: string
  checkedAt?: string
  status: ProjectTopologyStatus
  observationCode?: string
  checks: ServiceBindingCheckItem[]
}

export type ServiceBindingPage = PaginatedResponse<ServiceBinding>
export type ProjectTopologyManualEdgePage = PaginatedResponse<ProjectTopologyManualEdge>
