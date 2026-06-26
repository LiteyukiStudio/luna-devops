import type { GitAccount, GitProvider } from '@/api'
import i18next from 'i18next'
import { z } from 'zod'

export const providerSchema = z.object({
  name: z.string().min(1, i18next.t('codeRepositoriesView.providerNameRequired')),
  type: z.enum(['github', 'gitea', 'gitlab']),
  baseUrl: z.string().optional(),
  scope: z.enum(['global', 'project', 'user']),
  ownerRef: z.string(),
  projectIds: z.array(z.string()),
  authType: z.enum(['oauth', 'pat']),
  clientId: z.string().optional(),
  clientSecret: z.string().optional(),
  enabled: z.boolean(),
}).superRefine((value, ctx) => {
  if (value.scope === 'project' && value.projectIds.length === 0) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ['projectIds'],
      message: i18next.t('codeRepositoriesView.ownerProjectRequired'),
    })
  }
})

export const credentialSchema = z.object({
  providerId: z.string().min(1, i18next.t('codeRepositoriesView.providerRequired')),
  scope: z.enum(['global', 'project', 'user']),
  ownerRef: z.string(),
  projectIds: z.array(z.string()),
  username: z.string().min(1, i18next.t('codeRepositoriesView.usernameRequired')),
  externalUserId: z.string().optional(),
  avatarUrl: z.string().optional(),
  accessToken: z.string().optional(),
  refreshToken: z.string().optional(),
  scopesText: z.string().optional(),
  accessScope: z.enum(['personal', 'provider']),
  status: z.enum(['connected', 'expired', 'revoked']),
}).superRefine((value, ctx) => {
  if (value.scope === 'project' && value.projectIds.length === 0) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ['projectIds'],
      message: i18next.t('codeRepositoriesView.ownerProjectRequired'),
    })
  }
})

export type ProviderForm = z.infer<typeof providerSchema>
export type CredentialForm = z.infer<typeof credentialSchema>

export const providerDefaults: ProviderForm = {
  authType: 'oauth',
  baseUrl: 'https://github.com',
  ownerRef: '',
  projectIds: [],
  clientId: '',
  clientSecret: '',
  scope: 'global',
  enabled: true,
  name: '',
  type: 'github',
}

export const credentialDefaults: CredentialForm = {
  accessScope: 'personal',
  accessToken: '',
  avatarUrl: '',
  externalUserId: '',
  ownerRef: '',
  projectIds: [],
  providerId: '',
  refreshToken: '',
  scope: 'user',
  scopesText: 'repo,read:user',
  status: 'connected',
  username: '',
}

export type ProviderPayload = Omit<GitProvider, 'id' | 'createdAt' | 'clientSecretSet'> & {
  scope?: GitProvider['scope']
  ownerRef?: string
  clientSecret?: string
}

export type CredentialPayload = Omit<GitAccount, 'id' | 'userId' | 'scopes' | 'createdAt' | 'accessTokenSet' | 'refreshTokenSet'> & {
  scope?: GitAccount['scope']
  ownerRef?: string
  scopes: string[]
  accessToken?: string
  refreshToken?: string
}
