import type { components } from './generated/openapi.js'
import type {
  Application,
  ApplicationCurrentRelease,
  AppTemplate,
  AppTemplateComponent,
  AppTemplateConfigFile,
  AppTemplateSummary,
  DeploymentTargetConfigInferenceInput,
  DeploymentTargetConfigInferenceOutput,
  DeploymentTargetMetrics,
  DeploymentTargetPodMetrics,
  PlatformAppTemplate,
  PlatformAppTemplateBundle,
  PlatformAppTemplateComponent,
  PlatformAppTemplateWritePayload,
} from './types'
import { describe, expectTypeOf, it } from 'vitest'

describe('application list transport contracts', () => {
  it('keeps current release, access URL, and deployment target metrics bound to OpenAPI', () => {
    expectTypeOf<ApplicationCurrentRelease>().toEqualTypeOf<components['schemas']['ApplicationCurrentRelease']>()
    expectTypeOf<Application['primaryAccessUrl']>().toEqualTypeOf<components['schemas']['ApplicationListItem']['primaryAccessUrl']>()
    expectTypeOf<DeploymentTargetPodMetrics>().toEqualTypeOf<components['schemas']['DeploymentTargetPodMetrics']>()
    expectTypeOf<DeploymentTargetMetrics>().toEqualTypeOf<components['schemas']['DeploymentTargetMetrics']>()
    expectTypeOf<DeploymentTargetConfigInferenceInput>().toEqualTypeOf<components['schemas']['DeploymentTargetConfigInferenceInput']>()
    expectTypeOf<DeploymentTargetConfigInferenceOutput>().toEqualTypeOf<components['schemas']['DeploymentTargetConfigInferenceOutput']>()
  })

  it('keeps the app template featured flag, source, and platform schemas bound to OpenAPI', () => {
    expectTypeOf<AppTemplateSummary['featured']>().toEqualTypeOf<components['schemas']['AppTemplateSummary']['featured']>()
    expectTypeOf<AppTemplate['featured']>().toEqualTypeOf<components['schemas']['AppTemplate']['featured']>()
    expectTypeOf<NonNullable<AppTemplateSummary['source']>>().toEqualTypeOf<components['schemas']['AppTemplateSummary']['source']>()
    expectTypeOf<NonNullable<AppTemplateComponent['dependencies']>>().toEqualTypeOf<components['schemas']['AppTemplateComponent']['dependencies']>()
    expectTypeOf<AppTemplateConfigFile>().toEqualTypeOf<components['schemas']['AppTemplateConfigFile']>()
    expectTypeOf<Omit<PlatformAppTemplateComponent, 'dataVolumes'>>().toEqualTypeOf<Omit<components['schemas']['PlatformAppTemplateComponent'], 'dataVolumes'>>()
    expectTypeOf<Omit<PlatformAppTemplateWritePayload, 'dataVolumes' | 'components'>>().toEqualTypeOf<Omit<components['schemas']['PlatformAppTemplateWriteInput'], 'dataVolumes' | 'components'>>()
    expectTypeOf<Omit<PlatformAppTemplateBundle, 'dataVolumes' | 'components'>>().toEqualTypeOf<Omit<components['schemas']['PlatformAppTemplateBundle'], 'dataVolumes' | 'components'>>()
    expectTypeOf<Omit<PlatformAppTemplate, 'dataVolumes' | 'components'>>().toEqualTypeOf<Omit<components['schemas']['PlatformAppTemplate'], 'dataVolumes' | 'components'>>()
  })
})
