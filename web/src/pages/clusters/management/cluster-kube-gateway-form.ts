export const kubeGatewayVerbOptions = ['get', 'list', 'watch', 'create', 'update', 'patch', 'delete', 'deletecollection', 'connect'] as const

export const kubeGatewayActionOptions = [
  'project:read',
  'deployment:read',
  'deployment:update',
  'deployment:restart',
  'deployment:delete',
  'deployment:exec',
  'secret:read_summary',
  'secret:view_value',
  'secret:update',
  'volume:read',
  'volume:write',
  'volume:delete',
  'gateway:read',
  'gateway:manage',
  'cluster:read',
  'cluster:manage',
] as const

export interface ClusterKubeGatewayRuleFormValue {
  action: string
  apiGroup: string
  apiVersion: string
  resource: string
  subresourcesText: string
  verbs: string[]
}

export interface ClusterKubeGatewayFormValues {
  enabled: boolean
  extraResourceRules: ClusterKubeGatewayRuleFormValue[]
}

export function createEmptyClusterKubeGatewayRule(): ClusterKubeGatewayRuleFormValue {
  return {
    action: 'deployment:read',
    apiGroup: '',
    apiVersion: 'v1',
    resource: '',
    subresourcesText: '',
    verbs: ['get', 'list', 'watch'],
  }
}
