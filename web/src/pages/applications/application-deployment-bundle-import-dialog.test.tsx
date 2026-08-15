import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApplicationDeploymentBundleImportDialog } from './application-deployment-bundle-import-dialog'
import '@/i18n'

const { importDeploymentTargetBundle, previewDeploymentTargetBundleImport } = vi.hoisted(() => ({
  importDeploymentTargetBundle: vi.fn(),
  previewDeploymentTargetBundleImport: vi.fn(),
}))

vi.mock('@/api', () => ({
  api: {
    importDeploymentTargetBundle,
    previewDeploymentTargetBundleImport,
  },
}))

const bundle = {
  configuration: {
    dataVolumes: [],
    name: 'Imported service',
    namespace: '',
    runtimeConfigRefs: [],
    runtimeConfigSetIds: [],
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
    previewDeploymentTargetBundleImport.mockResolvedValue({
      digest: 'a'.repeat(64),
      references: [],
      secretRequirements: [],
      status: 'ready',
      summary: { name: 'Imported service', namespace: '', sourceType: 'image', stage: 'dev' },
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
      expect.objectContaining({ bundle, mappings: {}, overrides: expect.objectContaining({ stage: 'dev' }) }),
    ))

    const importButton = await screen.findByRole('button', { name: /Import config|导入配置/ })
    expect(importButton).toBeEnabled()
    fireEvent.click(importButton)

    await waitFor(() => expect(importDeploymentTargetBundle).toHaveBeenCalledWith(
      'prj_destination',
      'app_destination',
      expect.objectContaining({ bundle, digest: 'a'.repeat(64), secretValues: {} }),
    ))
    expect(onImported).toHaveBeenCalledOnce()
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
      summary: { name: 'Imported service', namespace: '', sourceType: 'image', stage: 'dev' },
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
})
