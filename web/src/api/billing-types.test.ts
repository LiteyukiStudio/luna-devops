import type { components } from './generated/openapi.js'
import type {
  BillingSpendCategory,
  BillingSpendTimeseries,
  BillingSpendTimeseriesBucket,
  BillingSummary,
} from './types'
import { describe, expectTypeOf, it } from 'vitest'

describe('billing transport contracts', () => {
  it('keeps summary and spend timeseries schemas bound to OpenAPI', () => {
    expectTypeOf<BillingSpendCategory>().toEqualTypeOf<components['schemas']['BillingSpendCategory']>()
    expectTypeOf<BillingSpendTimeseriesBucket>().toEqualTypeOf<components['schemas']['BillingSpendTimeseriesBucket']>()
    expectTypeOf<BillingSpendTimeseries>().toEqualTypeOf<components['schemas']['BillingSpendTimeseries']>()
    expectTypeOf<BillingSummary>().toEqualTypeOf<components['schemas']['BillingSummary']>()
  })
})
