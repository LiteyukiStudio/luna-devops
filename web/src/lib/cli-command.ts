export type CliInvocationSpec
  = | {
    command: 'deployment.exec'
    server: string
    projectId: string
    applicationId: string
    targetId: string
    container?: string
  }
  | {
    command: 'cluster.pod-terminal'
    server: string
    clusterId: string
    namespace: string
    name: string
    container?: string
  }

const SAFE_SHELL_TOKEN = /^[\w./:@%+=,-]+$/

function shellQuoteToken(token: string) {
  if (SAFE_SHELL_TOKEN.test(token))
    return token
  return `'${token.replaceAll('\'', '\'"\'"\'')}'`
}

function argument(name: string, value: string) {
  return shellQuoteToken(`${name}=${value}`)
}

function optionalArgument(name: string, value?: string) {
  const normalized = value?.trim()
  return normalized ? argument(name, normalized) : undefined
}

export function buildCliCommand(spec: CliInvocationSpec) {
  const tokens = spec.command === 'deployment.exec'
    ? [
        'luna',
        'deployment',
        'exec',
        argument('server', spec.server),
        argument('projectId', spec.projectId),
        argument('applicationId', spec.applicationId),
        argument('targetId', spec.targetId),
        optionalArgument('container', spec.container),
      ]
    : [
        'luna',
        'cluster',
        'pod-terminal',
        argument('server', spec.server),
        argument('clusterId', spec.clusterId),
        argument('namespace', spec.namespace),
        argument('name', spec.name),
        optionalArgument('container', spec.container),
      ]

  return tokens.filter((token): token is string => Boolean(token)).join(' ')
}
