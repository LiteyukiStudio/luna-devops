import type { TFunction } from 'i18next'
import type {
  AppTemplateConfigFile,
  AppTemplateDataVolume,
  AppTemplateValueDefinition,
  PlatformAppTemplate,
  PlatformAppTemplateBundle,
  PlatformAppTemplateComponentInput,
  PlatformAppTemplateWritePayload,
} from '@/api'
import { z } from 'zod'

export const PLATFORM_TEMPLATE_CATEGORIES = [
  'collaboration',
  'database',
  'developerTool',
  'middleware',
  'observability',
  'security',
  'storage',
] as const

const SLUG_PATTERN = /^[a-z0-9](?:[a-z0-9-]{0,46}[a-z0-9])?$/
const COMPONENT_KEY_PATTERN = /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/
const PARAM_KEY_PATTERN = /^[a-z]\w*$/i
const HTTP_URL_PATTERN = /^https?:\/\/\S+$/i

const configFileSchema = z.object({
  content: z.string(),
  path: z.string(),
})

const dataVolumeSchema = z.discriminatedUnion('sourceType', [
  z.object({
    devicePath: z.string().optional(),
    logicalName: z.string(),
    mountPath: z.string().optional(),
    readOnly: z.boolean().optional(),
    sourceType: z.literal('projectVolume'),
  }),
  z.object({
    emptyDir: z.object({
      medium: z.enum(['', 'Memory']).optional(),
      sizeLimit: z.string().optional(),
    }).optional(),
    logicalName: z.string(),
    mountPath: z.string(),
    sourceType: z.literal('emptyDir'),
  }),
])

export function createPlatformTemplateFormSchema(t: TFunction) {
  const componentItemSchema = z.object({
    configFiles: z.array(configFileSchema),
    containerArgs: z.string(),
    containerCommand: z.string(),
    defaultCPU: z.string(),
    defaultMemory: z.string(),
    defaultReplicas: z.number(),
    envLines: z.string(),
    extraDataVolumes: z.array(dataVolumeSchema),
    fsGroup: z.string(),
    image: z.string(),
    key: z.string(),
    name: z.string(),
    persistentLogicalName: z.string(),
    persistentMountPath: z.string(),
    secretEnvLines: z.string(),
    secretFiles: z.array(configFileSchema),
    servicePort: z.number(),
  })

  const parameterItemSchema = z.object({
    autoGenerate: z.boolean(),
    defaultValue: z.string(),
    description: z.string(),
    key: z.string(),
    label: z.string(),
    required: z.boolean(),
    secret: z.boolean(),
  })

  return z.object({
    category: z.string(),
    components: z.array(componentItemSchema),
    configFiles: z.array(configFileSchema),
    containerArgs: z.string(),
    containerCommand: z.string(),
    defaultCPU: z.string(),
    defaultMemory: z.string(),
    defaultReplicas: z.number(),
    description: z.string(),
    enabled: z.boolean(),
    envLines: z.string(),
    extraDataVolumes: z.array(dataVolumeSchema),
    featured: z.boolean(),
    fsGroup: z.string(),
    icon: z.string(),
    image: z.string(),
    name: z.string(),
    officialRepository: z.string(),
    officialWebsite: z.string(),
    persistentLogicalName: z.string(),
    persistentMountPath: z.string(),
    popularityWeight: z.number(),
    secretEnvLines: z.string(),
    secretFiles: z.array(configFileSchema),
    servicePort: z.number(),
    slug: z.string(),
    values: z.array(parameterItemSchema),
    version: z.string(),
    workloadMode: z.enum(['single', 'multi']),
  }).superRefine((value, ctx) => {
    const issue = (path: (string | number)[], message: string) => ctx.addIssue({ code: 'custom', message, path })

    if (!SLUG_PATTERN.test(value.slug.trim()))
      issue(['slug'], t('appTemplatesPage.platformAdmin.validation.slugInvalid'))
    if (!value.name.trim())
      issue(['name'], t('appTemplatesPage.platformAdmin.validation.nameRequired'))
    if (value.officialWebsite.trim() && !HTTP_URL_PATTERN.test(value.officialWebsite.trim()))
      issue(['officialWebsite'], t('appTemplatesPage.platformAdmin.validation.urlInvalid'))
    if (value.officialRepository.trim() && !HTTP_URL_PATTERN.test(value.officialRepository.trim()))
      issue(['officialRepository'], t('appTemplatesPage.platformAdmin.validation.urlInvalid'))

    if (value.workloadMode === 'single') {
      if (!value.image.trim())
        issue(['image'], t('appTemplatesPage.validation.imageRequired'))
      if (!Number.isInteger(value.servicePort) || value.servicePort < 1 || value.servicePort > 65535)
        issue(['servicePort'], t('appTemplatesPage.platformAdmin.validation.portInvalid'))
      if (!Number.isInteger(value.defaultReplicas) || value.defaultReplicas < 1)
        issue(['defaultReplicas'], t('appTemplatesPage.validation.replicasMin'))
      if (hasInvalidEnvLine(value.envLines))
        issue(['envLines'], t('appTemplatesPage.platformAdmin.validation.envLineInvalid'))
      if (hasInvalidEnvLine(value.secretEnvLines))
        issue(['secretEnvLines'], t('appTemplatesPage.platformAdmin.validation.envLineInvalid'))
    }
    else {
      if (value.components.length < 2)
        issue(['components'], t('appTemplatesPage.platformAdmin.validation.componentsMin'))
      const seenKeys = new Set<string>()
      value.components.forEach((component, index) => {
        const key = component.key.trim()
        if (!COMPONENT_KEY_PATTERN.test(key))
          issue(['components', index, 'key'], t('appTemplatesPage.platformAdmin.validation.componentKeyInvalid'))
        else if (seenKeys.has(key))
          issue(['components', index, 'key'], t('appTemplatesPage.platformAdmin.validation.componentKeyDuplicate'))
        else
          seenKeys.add(key)

        if (!component.name.trim())
          issue(['components', index, 'name'], t('appTemplatesPage.validation.applicationNameRequired'))
        if (!component.image.trim())
          issue(['components', index, 'image'], t('appTemplatesPage.validation.imageRequired'))
        if (!Number.isInteger(component.servicePort) || component.servicePort < 1 || component.servicePort > 65535)
          issue(['components', index, 'servicePort'], t('appTemplatesPage.platformAdmin.validation.portInvalid'))
        if (!Number.isInteger(component.defaultReplicas) || component.defaultReplicas < 1)
          issue(['components', index, 'defaultReplicas'], t('appTemplatesPage.validation.replicasMin'))
        if (hasInvalidEnvLine(component.envLines))
          issue(['components', index, 'envLines'], t('appTemplatesPage.platformAdmin.validation.envLineInvalid'))
        if (hasInvalidEnvLine(component.secretEnvLines))
          issue(['components', index, 'secretEnvLines'], t('appTemplatesPage.platformAdmin.validation.envLineInvalid'))
      })
    }

    const seenParamKeys = new Set<string>()
    value.values.forEach((param, index) => {
      const key = param.key.trim()
      if (!PARAM_KEY_PATTERN.test(key))
        issue(['values', index, 'key'], t('appTemplatesPage.platformAdmin.validation.paramKeyInvalid'))
      else if (seenParamKeys.has(key))
        issue(['values', index, 'key'], t('appTemplatesPage.platformAdmin.validation.paramKeyDuplicate'))
      else
        seenParamKeys.add(key)

      if (param.secret && param.defaultValue.trim() !== '')
        issue(['values', index, 'defaultValue'], t('appTemplatesPage.platformAdmin.validation.secretDefaultForbidden'))
    })
  })
}

export type PlatformTemplateFormValues = z.infer<ReturnType<typeof createPlatformTemplateFormSchema>>
export type PlatformTemplateComponentFormValue = PlatformTemplateFormValues['components'][number]
export type PlatformTemplateParameterFormValue = PlatformTemplateFormValues['values'][number]

export function emptyPlatformComponentFormValue(index = 1): PlatformTemplateComponentFormValue {
  return {
    configFiles: [],
    containerArgs: '',
    containerCommand: '',
    defaultCPU: '250m',
    defaultMemory: '256Mi',
    defaultReplicas: 1,
    envLines: '',
    extraDataVolumes: [],
    fsGroup: '',
    image: '',
    key: index === 1 ? 'web' : index === 2 ? 'db' : `svc-${index}`,
    name: '',
    persistentLogicalName: 'data',
    persistentMountPath: '',
    secretEnvLines: '',
    secretFiles: [],
    servicePort: 8080,
  }
}

export function emptyPlatformParameterFormValue(): PlatformTemplateParameterFormValue {
  return {
    autoGenerate: false,
    defaultValue: '',
    description: '',
    key: '',
    label: '',
    required: false,
    secret: false,
  }
}

export function defaultPlatformTemplateFormValues(template?: PlatformAppTemplate | null): PlatformTemplateFormValues {
  if (!template) {
    return {
      category: 'developerTool',
      components: [emptyPlatformComponentFormValue(1), emptyPlatformComponentFormValue(2)],
      configFiles: [],
      containerArgs: '',
      containerCommand: '',
      defaultCPU: '250m',
      defaultMemory: '256Mi',
      defaultReplicas: 1,
      description: '',
      enabled: true,
      envLines: '',
      extraDataVolumes: [],
      featured: false,
      fsGroup: '',
      icon: '',
      image: '',
      name: '',
      officialRepository: '',
      officialWebsite: '',
      persistentLogicalName: 'data',
      persistentMountPath: '',
      popularityWeight: 50,
      secretEnvLines: '',
      secretFiles: [],
      servicePort: 8080,
      slug: '',
      values: [],
      version: '1.0',
      workloadMode: 'single',
    }
  }

  const singleVolumes = splitPrimaryProjectVolume(template.dataVolumes)
  const mappedComponents: PlatformTemplateComponentFormValue[] = template.components.map((component) => {
    const componentVolumes = splitPrimaryProjectVolume(component.dataVolumes)
    return {
      configFiles: component.configFiles ?? [],
      containerArgs: component.containerArgs ?? '',
      containerCommand: component.containerCommand ?? '',
      defaultCPU: component.defaultCPU || '250m',
      defaultMemory: component.defaultMemory || '256Mi',
      defaultReplicas: component.defaultReplicas || 1,
      envLines: recordToEnvLines(component.env),
      extraDataVolumes: componentVolumes.extraDataVolumes,
      fsGroup: component.fsGroup ?? '',
      image: component.image ?? '',
      key: component.key ?? '',
      name: component.name ?? '',
      persistentLogicalName: componentVolumes.logicalName,
      persistentMountPath: componentVolumes.mountPath,
      secretEnvLines: recordToEnvLines(component.secretEnv),
      secretFiles: component.secretFiles ?? [],
      servicePort: component.servicePort || 8080,
    }
  })

  return {
    category: template.category || 'developerTool',
    components: mappedComponents.length >= 2 ? mappedComponents : [emptyPlatformComponentFormValue(1), emptyPlatformComponentFormValue(2)],
    configFiles: template.configFiles ?? [],
    containerArgs: template.containerArgs ?? '',
    containerCommand: template.containerCommand ?? '',
    defaultCPU: template.defaultCPU || '250m',
    defaultMemory: template.defaultMemory || '256Mi',
    defaultReplicas: template.defaultReplicas || 1,
    description: template.description ?? '',
    enabled: template.enabled,
    envLines: recordToEnvLines(template.env),
    extraDataVolumes: singleVolumes.extraDataVolumes,
    featured: template.featured,
    fsGroup: template.fsGroup ?? '',
    icon: template.icon ?? '',
    image: template.image ?? '',
    name: template.name ?? '',
    officialRepository: template.officialRepository ?? '',
    officialWebsite: template.officialWebsite ?? '',
    persistentLogicalName: singleVolumes.logicalName,
    persistentMountPath: singleVolumes.mountPath,
    popularityWeight: Number.isFinite(template.popularityWeight) ? template.popularityWeight : 50,
    secretEnvLines: recordToEnvLines(template.secretEnv),
    secretFiles: template.secretFiles ?? [],
    servicePort: template.servicePort || 8080,
    slug: template.slug ?? '',
    values: (template.values ?? []).map(param => ({
      autoGenerate: param.autoGenerate,
      defaultValue: param.secret ? '' : param.default,
      description: param.description ?? '',
      key: param.key,
      label: param.label ?? '',
      required: param.required,
      secret: param.secret,
    })),
    version: template.version ?? '',
    workloadMode: template.isMultiComponent ? 'multi' : 'single',
  }
}

export function toPlatformTemplateWritePayload(values: PlatformTemplateFormValues): PlatformAppTemplateWritePayload {
  const mappedValues: AppTemplateValueDefinition[] = values.values.map(param => ({
    autoGenerate: param.secret ? param.autoGenerate : false,
    default: param.secret ? '' : param.defaultValue.trim(),
    description: param.description.trim(),
    key: param.key.trim(),
    label: param.label.trim() || param.key.trim(),
    required: param.required,
    secret: param.secret,
  }))

  const base: PlatformAppTemplateWritePayload = {
    category: values.category.trim(),
    description: values.description.trim(),
    enabled: values.enabled,
    featured: values.featured,
    icon: values.icon.trim(),
    name: values.name.trim(),
    officialRepository: values.officialRepository.trim(),
    officialWebsite: values.officialWebsite.trim(),
    popularityWeight: Number.isFinite(values.popularityWeight) ? Math.max(0, Math.round(values.popularityWeight)) : 0,
    slug: values.slug.trim(),
    values: mappedValues,
    version: values.version.trim(),
  }

  if (values.workloadMode === 'multi') {
    const components: PlatformAppTemplateComponentInput[] = values.components.map(component => ({
      configFiles: cleanConfigFiles(component.configFiles),
      containerArgs: component.containerArgs.trim(),
      containerCommand: component.containerCommand.trim(),
      dataVolumes: mergeDataVolumes(component.persistentLogicalName, component.persistentMountPath, component.extraDataVolumes),
      defaultCPU: component.defaultCPU.trim() || '250m',
      defaultMemory: component.defaultMemory.trim() || '256Mi',
      defaultReplicas: component.defaultReplicas || 1,
      env: envLinesToRecord(component.envLines),
      fsGroup: component.fsGroup.trim(),
      image: component.image.trim(),
      key: component.key.trim(),
      name: component.name.trim(),
      secretEnv: envLinesToRecord(component.secretEnvLines),
      secretFiles: cleanConfigFiles(component.secretFiles),
      servicePort: component.servicePort,
    }))
    return {
      ...base,
      components,
    }
  }

  return {
    ...base,
    components: [],
    configFiles: cleanConfigFiles(values.configFiles),
    containerArgs: values.containerArgs.trim(),
    containerCommand: values.containerCommand.trim(),
    dataVolumes: mergeDataVolumes(values.persistentLogicalName, values.persistentMountPath, values.extraDataVolumes),
    defaultCPU: values.defaultCPU.trim() || '250m',
    defaultMemory: values.defaultMemory.trim() || '256Mi',
    defaultReplicas: values.defaultReplicas || 1,
    env: envLinesToRecord(values.envLines),
    fsGroup: values.fsGroup.trim(),
    image: values.image.trim(),
    secretEnv: envLinesToRecord(values.secretEnvLines),
    secretFiles: cleanConfigFiles(values.secretFiles),
    servicePort: values.servicePort,
  }
}

export function platformTemplateToWritePayload(
  template: PlatformAppTemplate,
  overrides?: Partial<PlatformAppTemplateWritePayload>,
): PlatformAppTemplateWritePayload {
  const payload = toPlatformTemplateWritePayload(defaultPlatformTemplateFormValues(template))
  return { ...payload, ...overrides }
}

const bundleValueDefinitionSchema: z.ZodType<AppTemplateValueDefinition> = z.object({
  autoGenerate: z.boolean().default(false),
  default: z.string().default(''),
  description: z.string().default(''),
  key: z.string().trim().min(1),
  label: z.string().default(''),
  required: z.boolean().default(false),
  secret: z.boolean().default(false),
})

const bundleComponentInputSchema: z.ZodType<PlatformAppTemplateComponentInput> = z.object({
  configFiles: z.array(configFileSchema).optional(),
  containerArgs: z.string().optional(),
  containerCommand: z.string().optional(),
  dataVolumes: z.array(dataVolumeSchema).optional(),
  defaultCPU: z.string().optional(),
  defaultMemory: z.string().optional(),
  defaultReplicas: z.number().optional(),
  env: z.record(z.string(), z.string()).optional(),
  fsGroup: z.string().optional(),
  image: z.string(),
  key: z.string().trim().min(1),
  name: z.string().trim().min(1),
  secretEnv: z.record(z.string(), z.string()).optional(),
  secretFiles: z.array(configFileSchema).optional(),
  servicePort: z.number(),
})

const platformTemplateBundleSchema: z.ZodType<PlatformAppTemplateBundle> = z.object({
  category: z.string().trim().min(1),
  components: z.array(bundleComponentInputSchema).optional(),
  configFiles: z.array(configFileSchema).optional(),
  containerArgs: z.string().optional(),
  containerCommand: z.string().optional(),
  dataVolumes: z.array(dataVolumeSchema).optional(),
  defaultCPU: z.string().optional(),
  defaultMemory: z.string().optional(),
  defaultReplicas: z.number().optional(),
  description: z.string().optional(),
  enabled: z.boolean().optional(),
  env: z.record(z.string(), z.string()).optional(),
  featured: z.boolean().optional(),
  fsGroup: z.string().optional(),
  icon: z.string().optional(),
  image: z.string().optional(),
  name: z.string().trim().min(1),
  officialRepository: z.string().optional(),
  officialWebsite: z.string().optional(),
  popularityWeight: z.number().optional(),
  schemaVersion: z.string().optional(),
  secretEnv: z.record(z.string(), z.string()).optional(),
  secretFiles: z.array(configFileSchema).optional(),
  servicePort: z.number().optional(),
  slug: z.string().trim().min(1),
  values: z.array(bundleValueDefinitionSchema).optional(),
  version: z.string().optional(),
})

export function parsePlatformTemplateBundleJson(
  raw: string,
): { bundle: PlatformAppTemplateBundle } | { errorCode: 'bundleInvalid' | 'jsonEmpty' | 'jsonInvalid' } {
  const trimmed = raw.trim()
  if (!trimmed)
    return { errorCode: 'jsonEmpty' }

  let parsed: unknown
  try {
    parsed = JSON.parse(trimmed)
  }
  catch {
    return { errorCode: 'jsonInvalid' }
  }

  if (!isPlainRecord(parsed))
    return { errorCode: 'bundleInvalid' }

  // Support both bare bundle JSON and `{ bundle: { ... } }` wrapper.
  const candidate = isPlainRecord(parsed.bundle) ? parsed.bundle : parsed
  const validated = platformTemplateBundleSchema.safeParse(candidate)
  if (!validated.success)
    return { errorCode: 'bundleInvalid' }

  return { bundle: validated.data }
}

export function downloadTemplateBundleJson(bundle: PlatformAppTemplateBundle) {
  const slug = (bundle.slug || 'app-template').replace(/[^a-z0-9-]+/gi, '-').replace(/^-+|-+$/g, '') || 'app-template'
  const content = `${JSON.stringify(bundle, null, 2)}\n`
  const blob = new Blob([content], { type: 'application/json;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = `${slug}.template.json`
  document.body.append(anchor)
  anchor.click()
  anchor.remove()
  URL.revokeObjectURL(url)
}

function splitPrimaryProjectVolume(volumes: AppTemplateDataVolume[] | undefined): {
  extraDataVolumes: AppTemplateDataVolume[]
  logicalName: string
  mountPath: string
} {
  const list = volumes ?? []
  const primaryIndex = list.findIndex(volume => volume.sourceType === 'projectVolume' && Boolean(volume.mountPath) && !volume.devicePath && !volume.readOnly)
  if (primaryIndex < 0) {
    return { extraDataVolumes: list, logicalName: 'data', mountPath: '' }
  }
  const primary = list[primaryIndex]
  return {
    extraDataVolumes: list.filter((_, index) => index !== primaryIndex),
    logicalName: primary.logicalName || 'data',
    mountPath: primary.sourceType === 'projectVolume' ? (primary.mountPath ?? '') : '',
  }
}

function mergeDataVolumes(logicalName: string, mountPath: string, extraDataVolumes: AppTemplateDataVolume[]): AppTemplateDataVolume[] {
  const normalizedMountPath = mountPath.trim()
  if (!normalizedMountPath)
    return extraDataVolumes
  return [
    {
      logicalName: logicalName.trim() || 'data',
      mountPath: normalizedMountPath,
      sourceType: 'projectVolume',
    },
    ...extraDataVolumes,
  ]
}

function cleanConfigFiles(files: AppTemplateConfigFile[]): AppTemplateConfigFile[] {
  return files.filter(file => file.path.trim() !== '')
}

function hasInvalidEnvLine(raw: string): boolean {
  for (const line of raw.split(/\r?\n/)) {
    const trimmed = line.trim()
    if (!trimmed)
      continue
    const eqIndex = trimmed.indexOf('=')
    if (eqIndex <= 0)
      return true
  }
  return false
}

function envLinesToRecord(raw: string): Record<string, string> {
  const result: Record<string, string> = {}
  for (const line of raw.split(/\r?\n/)) {
    const trimmed = line.trim()
    if (!trimmed)
      continue
    const eqIndex = trimmed.indexOf('=')
    if (eqIndex <= 0)
      continue
    const key = trimmed.slice(0, eqIndex).trim()
    const value = trimmed.slice(eqIndex + 1).trim()
    if (key)
      result[key] = value
  }
  return result
}

function recordToEnvLines(record: Record<string, string> | undefined): string {
  if (!record)
    return ''
  return Object.entries(record)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, value]) => `${key}=${value}`)
    .join('\n')
}

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}
