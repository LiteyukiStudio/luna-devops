import type { components } from './generated/openapi.js'
import type { Application, ApplicationCurrentRelease, AppTemplate, AppTemplateSummary } from './types'
import { describe, expectTypeOf, it } from 'vitest'

describe('application list transport contracts', () => {
  it('keeps current release and access URL bound to OpenAPI', () => {
    expectTypeOf<ApplicationCurrentRelease>().toEqualTypeOf<components['schemas']['ApplicationCurrentRelease']>()
    expectTypeOf<Application['primaryAccessUrl']>().toEqualTypeOf<components['schemas']['ApplicationListItem']['primaryAccessUrl']>()
  })

  it('keeps the app template featured flag bound to OpenAPI', () => {
    expectTypeOf<AppTemplateSummary['featured']>().toEqualTypeOf<components['schemas']['AppTemplateSummary']['featured']>()
    expectTypeOf<AppTemplate['featured']>().toEqualTypeOf<components['schemas']['AppTemplate']['featured']>()
  })
})
