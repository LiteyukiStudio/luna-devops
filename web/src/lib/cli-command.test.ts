import { describe, expect, it } from 'vitest'
import { buildCliCommand } from './cli-command'

describe('buildCliCommand', () => {
  it('keeps deployment arguments in a stable order and omits an empty container', () => {
    expect(buildCliCommand({
      command: 'deployment.exec',
      server: 'https://devops.example.com',
      projectId: 'prj_123',
      applicationId: 'app_456',
      targetId: 'dplt_789',
      container: '  ',
    })).toBe('luna deployment exec server=https://devops.example.com projectId=prj_123 applicationId=app_456 targetId=dplt_789')
  })

  it('builds the existing cluster pod terminal command in a stable order', () => {
    expect(buildCliCommand({
      command: 'cluster.pod-terminal',
      server: 'https://devops.example.com',
      clusterId: 'clu_123',
      namespace: 'luna-project',
      name: 'api-7f6cb48d8f-j9km2',
      container: 'app',
    })).toBe('luna cluster pod-terminal server=https://devops.example.com clusterId=clu_123 namespace=luna-project name=api-7f6cb48d8f-j9km2 container=app')
  })

  it('quotes special characters as one shell token', () => {
    expect(buildCliCommand({
      command: 'cluster.pod-terminal',
      server: 'https://devops.example.com',
      clusterId: 'clu_123',
      namespace: 'default',
      name: 'pod-a',
      container: 'worker; echo \'not a command\' $(whoami)',
    })).toBe(`luna cluster pod-terminal server=https://devops.example.com clusterId=clu_123 namespace=default name=pod-a 'container=worker; echo '"'"'not a command'"'"' $(whoami)'`)
  })
})
