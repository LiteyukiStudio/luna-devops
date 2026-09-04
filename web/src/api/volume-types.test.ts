import type { components, operations } from './generated/openapi.js'
import type {
  PaginatedProjectVolumes,
  ProjectVolume,
  ProjectVolumeCreateInput,
  ProjectVolumeDetail,
  ProjectVolumeListParams,
  VolumeExportCreateInput,
  VolumeImportCreateInput,
  VolumeTransfer,
} from './volume-types'
import { describe, expectTypeOf, it } from 'vitest'

describe('generated volume transport contracts', () => {
  it('keeps public aliases bound to their OpenAPI operations', () => {
    expectTypeOf<ProjectVolume>().toEqualTypeOf<operations['createProjectVolume']['responses'][202]['content']['application/json']>()
    expectTypeOf<ProjectVolumeDetail>().toEqualTypeOf<operations['getProjectVolume']['responses'][200]['content']['application/json']>()
    expectTypeOf<PaginatedProjectVolumes>().toEqualTypeOf<operations['listProjectVolumes']['responses'][200]['content']['application/json']>()
    expectTypeOf<ProjectVolumeCreateInput>().toEqualTypeOf<operations['createProjectVolume']['requestBody']['content']['application/json']>()
    expectTypeOf<VolumeImportCreateInput>().toEqualTypeOf<operations['createVolumeImport']['requestBody']['content']['application/json']>()
    expectTypeOf<VolumeExportCreateInput>().toEqualTypeOf<operations['createVolumeExport']['requestBody']['content']['application/json']>()
    expectTypeOf<VolumeTransfer>().toEqualTypeOf<components['schemas']['VolumeTransfer']>()
    expectTypeOf<ProjectVolumeListParams>().toMatchTypeOf<NonNullable<operations['listProjectVolumes']['parameters']['query']>>()
  })
})
