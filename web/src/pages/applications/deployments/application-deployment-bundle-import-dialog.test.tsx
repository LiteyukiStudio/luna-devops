import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApplicationDeploymentBundleImportDialog } from './application-deployment-bundle-import-dialog'
import '@/i18n'

const { importDeploymentTargetBundle, listDeploymentTargetBundleReferenceCandidates, previewDeploymentTargetBundleImport } = vi.hoisted(() => ({
  importDeploymentTargetBundle: vi.fn(),
  listDeploymentTargetBundleReferenceCandidates: vi.fn(),
  previewDeploymentTargetBundleImport: vi.fn(),
}))

vi.mock('@/api', () => ({
  api: {
    importDeploymentTargetBundle,
    listDeploymentTargetBundleReferenceCandidates,
    previewDeploymentTargetBundleImport,
  },
}))

const bundle = {
  configuration: {
    dataVolumes: [],
    name: 'Imported service',
    runtimeConfigRefs: [],
    buildVariableSetIds: [],
    sourceType: 'image',
    imageRef: 'registry.example/service:v1',
    stage: 'dev',
  },
  exportedAt: '2026-08-16T00:00:00Z',
  kind: 'luna-devops.deployment-target',
  omissions: ['secretValues'],
  references: [],
  schemaVersion: 1,
  secretRequirements: [],
} as const

describe('deployment bundle import dialog', () => {
  beforeEach(() => {
    previewDeploymentTargetBundleImport.mockReset()
    importDeploymentTargetBundle.mockReset()
    listDeploymentTargetBundleReferenceCandidates.mockReset()
    previewDeploymentTargetBundleImport.mockResolvedValue({
      digest: 'a'.repeat(64),
      references: [],
      secretRequirements: [],
      status: 'ready',
      summary: { name: 'Imported service', sourceType: 'image', stage: 'dev' },
      warnings: [],
    })
    importDeploymentTargetBundle.mockResolvedValue({ id: 'dplt_imported' })
  })

  it('previews a local JSON bundle before committing it', async () => {
    const onImported = vi.fn()
    const file = Object.assign(new File([JSON.stringify(bundle)], 'deployment.json', { type: 'application/json' }), {
      text: vi.fn().mockResolvedValue(JSON.stringify(bundle)),
    })
    render(
      <ApplicationDeploymentBundleImportDialog
        applicationId="app_destination"
        open
        projectId="prj_destination"
        onImported={onImported}
        onOpenChange={vi.fn()}
      />,
    )

    fireEvent.change(screen.getByLabelText(/Select deploy config JSON|选择部署配置 JSON/), { target: { files: [file] } })
    await waitFor(() => expect(previewDeploymentTargetBundleImport).toHaveBeenCalledWith(
      'prj_destination',
      'app_destination',
      expect.objectContaining({ bundle, mappings: {}, overrides: expect.not.objectContaining({ stage: expect.anything() }) }),
    ))

    const importButton = await screen.findByRole('button', { name: /Import config|导入配置/ })
    expect(importButton).toBeEnabled()
    fireEvent.click(importButton)

    await waitFor(() => expect(importDeploymentTargetBundle).toHaveBeenCalledWith(
      'prj_destination',
      'app_destination',
      expect.objectContaining({
        bundle,
        digest: 'a'.repeat(64),
        overrides: expect.not.objectContaining({ stage: expect.anything() }),
        secretValues: {},
      }),
    ))
    expect(onImported).toHaveBeenCalledOnce()
  })

  it('keeps an invalid source stage and repairs it only after an explicit preflight override', async () => {
    const invalidBundle = { ...bundle, configuration: { ...bundle.configuration, stage: 'qa' } }
    previewDeploymentTargetBundleImport
      .mockResolvedValueOnce({
        digest: 'e'.repeat(64),
        references: [],
        secretRequirements: [],
        status: 'invalid',
        summary: { name: 'Imported service', sourceType: 'image', stage: 'qa' },
        warnings: ['deployment.stage_invalid'],
      })
      .mockResolvedValueOnce({
        digest: 'f'.repeat(64),
        references: [],
        secretRequirements: [],
        status: 'ready',
        summary: { name: 'Imported service', sourceType: 'image', stage: 'dev' },
        warnings: [],
      })
    const file = Object.assign(new File([JSON.stringify(invalidBundle)], 'legacy.json', { type: 'application/json' }), {
      text: vi.fn().mockResolvedValue(JSON.stringify(invalidBundle)),
    })
    render(
      <ApplicationDeploymentBundleImportDialog applicationId="app_destination" open projectId="prj_destination" onImported={vi.fn()} onOpenChange={vi.fn()} />,
    )

    fireEvent.change(screen.getByLabelText(/Select deploy config JSON|选择部署配置 JSON/), { target: { files: [file] } })
    await waitFor(() => expect(previewDeploymentTargetBundleImport).toHaveBeenNthCalledWith(
      1,
      'prj_destination',
      'app_destination',
      expect.objectContaining({ overrides: expect.not.objectContaining({ stage: expect.anything() }) }),
    ))
    expect(await screen.findByText(/stage must be dev, test, staging, or prod|部署阶段只允许 dev、test、staging 或 prod/)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /Run preflight again|重新预检/ }))
    await waitFor(() => expect(previewDeploymentTargetBundleImport).toHaveBeenNthCalledWith(
      2,
      'prj_destination',
      'app_destination',
      expect.objectContaining({ overrides: expect.objectContaining({ stage: 'dev' }) }),
    ))
    expect(screen.getByRole('button', { name: /Import config|导入配置/ })).toBeEnabled()
  })

  it('rejects files larger than the server contract before upload', async () => {
    const file = Object.assign(new File([new Uint8Array(1024 * 1024 + 1)], 'large.json', { type: 'application/json' }), {
      text: vi.fn(),
    })
    render(
      <ApplicationDeploymentBundleImportDialog
        applicationId="app_destination"
        open
        projectId="prj_destination"
        onImported={vi.fn()}
        onOpenChange={vi.fn()}
      />,
    )

    fireEvent.change(screen.getByLabelText(/Select deploy config JSON|选择部署配置 JSON/), { target: { files: [file] } })
    expect(await screen.findByText(/The file must be no larger than 1 MiB.|文件不能超过 1 MiB。/)).toBeInTheDocument()
    expect(previewDeploymentTargetBundleImport).not.toHaveBeenCalled()
  })

  it('keeps reference mappings visible while requiring a fresh preflight', async () => {
    Object.defineProperty(Element.prototype, 'scrollIntoView', { configurable: true, value: vi.fn() })
    previewDeploymentTargetBundleImport.mockResolvedValueOnce({
      digest: 'b'.repeat(64),
      references: [{
        candidateCount: 1,
        candidates: [{ compatible: true, id: 'reg_destination', matched: false, name: 'Destination registry' }],
        code: 'deployment_bundle.reference_missing',
        key: 'registry:target',
        kind: 'artifactRegistry',
        required: true,
        source: { name: 'Source registry' },
        status: 'missing',
        truncated: false,
        usage: 'targetImage',
      }],
      secretRequirements: [],
      status: 'requires_mapping',
      summary: { name: 'Imported service', sourceType: 'image', stage: 'dev' },
      warnings: ['deployment_bundle.reference_missing'],
    })
    const file = Object.assign(new File([JSON.stringify(bundle)], 'deployment.json', { type: 'application/json' }), {
      text: vi.fn().mockResolvedValue(JSON.stringify(bundle)),
    })
    render(
      <ApplicationDeploymentBundleImportDialog
        applicationId="app_destination"
        open
        projectId="prj_destination"
        onImported={vi.fn()}
        onOpenChange={vi.fn()}
      />,
    )

    fireEvent.change(screen.getByLabelText(/Select deploy config JSON|选择部署配置 JSON/), { target: { files: [file] } })
    const mapping = await screen.findByRole('combobox', { name: /Source registry/ })
    fireEvent.click(mapping)
    fireEvent.click(await screen.findByRole('option', { name: /Destination registry/ }))

    expect(screen.getByRole('combobox', { name: /Source registry/ })).toBeInTheDocument()
    expect(screen.getAllByText(/Run preflight again|需要重新预检/).length).toBeGreaterThanOrEqual(2)
    expect(screen.getByRole('button', { name: /Import config|导入配置/ })).toBeDisabled()
  })

  it('keeps a later-page candidate selected across paging and sends search and sort parameters', async () => {
    Object.defineProperty(Element.prototype, 'scrollIntoView', { configurable: true, value: vi.fn() })
    const reference = {
      candidateCount: 150,
      candidates: Array.from({ length: 20 }, (_, index) => ({ compatible: true, id: `config_${String(index + 1).padStart(3, '0')}`, matched: false, name: `Config ${String(index + 1).padStart(3, '0')}` })),
      code: 'deployment_bundle.reference_missing',
      key: 'runtimeConfigSet:0',
      kind: 'runtimeConfigSet',
      required: true,
      source: { name: 'Source config' },
      status: 'missing',
      truncated: true,
      usage: 'runtimeConfig',
    }
    previewDeploymentTargetBundleImport.mockResolvedValueOnce({
      digest: 'c'.repeat(64),
      references: [reference],
      secretRequirements: [],
      status: 'requires_mapping',
      summary: { name: 'Imported service', sourceType: 'image', stage: 'dev' },
      warnings: ['deployment_bundle.reference_missing'],
    })
    listDeploymentTargetBundleReferenceCandidates.mockImplementation((_projectId, _applicationId, _payload, params) => {
      const start = (params.page - 1) * 20 + 1
      return Promise.resolve({
        items: Array.from({ length: 20 }, (_, index) => ({ compatible: true, id: `config_${String(start + index).padStart(3, '0')}`, matched: false, name: `Config ${String(start + index).padStart(3, '0')}` })),
        page: params.page,
        pageSize: 20,
        sortBy: params.sortBy,
        sortOrder: params.sortOrder,
        total: 150,
        totalPages: 8,
      })
    })
    const file = Object.assign(new File([JSON.stringify(bundle)], 'deployment.json', { type: 'application/json' }), {
      text: vi.fn().mockResolvedValue(JSON.stringify(bundle)),
    })
    render(
      <ApplicationDeploymentBundleImportDialog applicationId="app_destination" open projectId="prj_destination" onImported={vi.fn()} onOpenChange={vi.fn()} />,
    )
    fireEvent.change(screen.getByLabelText(/Select deploy config JSON|选择部署配置 JSON/), { target: { files: [file] } })
    for (let page = 2; page <= 6; page++) {
      const next = await screen.findByRole('button', { name: /Next candidate page|下一页候选资源/ })
      fireEvent.click(next)
      await waitFor(() => expect(listDeploymentTargetBundleReferenceCandidates).toHaveBeenLastCalledWith(
        'prj_destination',
        'app_destination',
        expect.objectContaining({ reference: expect.objectContaining({ key: 'runtimeConfigSet:0' }) }),
        expect.objectContaining({ page }),
      ))
    }
    const mapping = screen.getByRole('combobox', { name: /Source config/ })
    fireEvent.click(mapping)
    fireEvent.click(await screen.findByRole('option', { name: /Config 120/ }))
    fireEvent.click(screen.getByRole('button', { name: /Previous candidate page|上一页候选资源/ }))
    await waitFor(() => expect(listDeploymentTargetBundleReferenceCandidates).toHaveBeenLastCalledWith(
      'prj_destination',
      'app_destination',
      expect.anything(),
      expect.objectContaining({ page: 5 }),
    ))
    expect(screen.getByRole('combobox', { name: /Source config/ })).toHaveTextContent('Config 120')

    fireEvent.change(screen.getByLabelText(/Search candidates for Source config|搜索 Source config 的候选资源/), { target: { value: 'Config 120' } })
    fireEvent.click(screen.getByRole('button', { name: /Search candidates|搜索候选资源/ }))
    await waitFor(() => expect(listDeploymentTargetBundleReferenceCandidates).toHaveBeenLastCalledWith(
      'prj_destination',
      'app_destination',
      expect.anything(),
      expect.objectContaining({ page: 1, search: 'Config 120', sortBy: 'name', sortOrder: 'asc' }),
    ))

    previewDeploymentTargetBundleImport.mockResolvedValueOnce({
      digest: 'd'.repeat(64),
      references: [{ ...reference, candidates: [{ compatible: true, id: 'replacement_001', matched: false, name: 'Replacement 001' }] }],
      secretRequirements: [],
      status: 'requires_mapping',
      summary: { name: 'Imported service', sourceType: 'image', stage: 'dev' },
      warnings: ['deployment_bundle.reference_missing'],
    })
    const replacementFile = Object.assign(new File([JSON.stringify(bundle)], 'replacement.json', { type: 'application/json' }), {
      text: vi.fn().mockResolvedValue(JSON.stringify(bundle)),
    })
    fireEvent.change(screen.getByLabelText(/Select deploy config JSON|选择部署配置 JSON/), { target: { files: [replacementFile] } })
    await waitFor(() => expect(previewDeploymentTargetBundleImport).toHaveBeenCalledTimes(2))
    expect(screen.getByRole('combobox', { name: /Source config/ })).not.toHaveTextContent('Config 120')
    expect(screen.getByLabelText(/Search candidates for Source config|搜索 Source config 的候选资源/)).toHaveValue('')
  })
})
