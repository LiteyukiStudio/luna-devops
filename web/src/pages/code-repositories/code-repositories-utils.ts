import type { GitProvider } from '@/api'
import { apiBaseOrigin } from '@/api'

export interface GitProviderGuide {
  type: GitProvider['type']
  createUrl?: string
  appName: string
  homepageUrl: string
  callbackUrl: string
  scopes: string
  docsUrl: string
}

export function splitText(value?: string) {
  return (value ?? '').split(',').map(item => item.trim()).filter(Boolean)
}

export function gitProviderGuide(type: GitProvider['type'], baseUrl?: string, name?: string): GitProviderGuide {
  const normalizedBaseUrl = normalizeGitBaseUrl(type, baseUrl)
  const callbackUrl = `${apiBaseOrigin()}/api/v1/git/oauth/callback`
  const appName = name?.trim() || 'Liteyuki DevOps'
  if (type === 'gitea') {
    return {
      appName,
      callbackUrl,
      createUrl: normalizedBaseUrl ? `${normalizedBaseUrl}/user/settings/applications` : undefined,
      docsUrl: 'https://docs.gitea.com/development/oauth2-provider',
      homepageUrl: apiBaseOrigin(),
      scopes: 'read:repository, write:repository, read:user',
      type,
    }
  }
  if (type === 'gitlab') {
    return {
      appName,
      callbackUrl,
      docsUrl: 'https://docs.gitlab.com/integration/oauth_provider/',
      homepageUrl: apiBaseOrigin(),
      scopes: 'read_user, read_repository, write_repository',
      type,
    }
  }
  return {
    appName,
    callbackUrl,
    createUrl: 'https://github.com/settings/applications/new',
    docsUrl: 'https://docs.github.com/apps/oauth-apps/building-oauth-apps/creating-an-oauth-app',
    homepageUrl: apiBaseOrigin(),
    scopes: 'repo, read:user',
    type: 'github',
  }
}

export function normalizeGitBaseUrl(type: GitProvider['type'], baseUrl?: string) {
  const trimmed = baseUrl?.trim().replace(/\/+$/, '')
  if (trimmed)
    return trimmed
  return type === 'github' ? 'https://github.com' : ''
}
