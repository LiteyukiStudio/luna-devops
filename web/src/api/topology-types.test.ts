import type { components, operations } from './generated/openapi.js'
import type {
  ProjectTopologyQuery,
  ServiceBindingCheckItem,
  ServiceBindingCheckResult,
  ServiceBindingPayload,
} from './topology-types'
import { describe, expectTypeOf, it } from 'vitest'

describe('generated topology transport contracts', () => {
  it('keeps complete public transport aliases bound to OpenAPI', () => {
    expectTypeOf<ServiceBindingPayload>().toEqualTypeOf<operations['createServiceBinding']['requestBody']['content']['application/json']>()
    expectTypeOf<ServiceBindingCheckItem>().toEqualTypeOf<components['schemas']['ServiceBindingCheckItem']>()
    expectTypeOf<ServiceBindingCheckResult>().toEqualTypeOf<components['schemas']['ServiceBindingCheckResult']>()
    expectTypeOf<ProjectTopologyQuery>().toMatchTypeOf<NonNullable<operations['getProjectTopology']['parameters']['query']>>()
  })
})
