import type { TFunction } from 'i18next'

export const MCP_PROTECTED_RESOURCE_METADATA_PATH = '/.well-known/oauth-protected-resource/api/v1/mcp'

export function resolveMcpEndpointUrl(origin?: string) {
  const baseOrigin = (origin ?? window.location.origin).trim().replace(/\/+$/, '')
  return `${baseOrigin}/api/v1/mcp`
}

export function buildMcpAgentPrompt(
  t: TFunction,
  options: {
    accessToken?: string
    endpointUrl: string
  },
) {
  const trimmedToken = options.accessToken?.trim()
  if (trimmedToken) {
    return t('accountPage.mcp.promptWithTokenTemplate', {
      accessToken: trimmedToken,
      endpointUrl: options.endpointUrl,
    })
  }
  return t('accountPage.mcp.promptTemplate', {
    endpointUrl: options.endpointUrl,
  })
}

export function buildMcpServersJsonSnippet(endpointUrl: string) {
  return JSON.stringify(
    {
      mcpServers: {
        'luna-devops': {
          url: endpointUrl,
          headers: {
            Authorization: 'Bearer <YOUR_PAT>',
          },
        },
      },
    },
    null,
    2,
  )
}

export function buildClaudeMcpCliSnippet(endpointUrl: string) {
  return [
    `claude mcp add --transport http luna-devops ${endpointUrl} \\`,
    '  --header "Authorization: Bearer <YOUR_PAT>"',
  ].join('\n')
}
