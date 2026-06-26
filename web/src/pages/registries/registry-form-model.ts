import type { RegistryCredential } from '@/api'
import i18next from 'i18next'
import { z } from 'zod'

export const registrySchema = z.object({
  name: z.string().min(1, i18next.t('registriesPage.registryNameRequired')),
  provider: z.enum(['harbor', 'dockerhub', 'gitea-registry']),
  endpoint: z.string().url(i18next.t('registriesPage.validUrlRequired')),
  scope: z.enum(['global', 'project', 'user']),
  ownerRef: z.string(),
  projectIds: z.array(z.string()),
  isDefault: z.boolean(),
  capabilitiesText: z.string(),
}).superRefine((values, ctx) => {
  if (values.scope === 'project' && values.projectIds.length === 0) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ['projectIds'],
      message: i18next.t('projectSpaces.selectProject'),
    })
  }
})

export const credentialSchema = z.object({
  registryId: z.string().min(1, i18next.t('registriesPage.registryRequired')),
  name: z.string().min(1, i18next.t('registriesPage.credentialNameRequired')),
  username: z.string(),
  password: z.string(),
  token: z.string(),
  scope: z.enum(['push-pull', 'push', 'pull']),
  accessScope: z.enum(['personal', 'registry']),
}).refine(values => values.password.trim() !== '' || values.token.trim() !== '', {
  message: i18next.t('registriesPage.passwordOrTokenRequired'),
  path: ['password'],
})

export const imageSchema = z.object({
  projectId: z.string(),
  applicationId: z.string(),
  registryId: z.string().min(1, i18next.t('registriesPage.registryRequired')),
  repository: z.string().min(1, i18next.t('registriesPage.repositoryRequired')),
  tag: z.string(),
  digest: z.string(),
  sourceCommit: z.string(),
  buildRunId: z.string(),
  sourceType: z.enum(['manual-image', 'build']),
  scanStatus: z.enum(['unknown', 'pending', 'scanning', 'passed', 'failed']),
})

export type RegistryForm = z.infer<typeof registrySchema>
export type CredentialForm = z.infer<typeof credentialSchema>
export type ImageForm = z.infer<typeof imageSchema>
export type CredentialWithRegistry = RegistryCredential & { registryName: string }

export const registryDefaults: RegistryForm = {
  name: '',
  provider: 'harbor',
  endpoint: '',
  scope: 'global',
  ownerRef: '',
  projectIds: [],
  isDefault: false,
  capabilitiesText: 'push,pull,tags,digest',
}

export function splitText(value: string) {
  return value.split(/[\n,]/).map(item => item.trim()).filter(Boolean)
}
