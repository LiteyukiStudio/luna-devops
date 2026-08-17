import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { emptyRuntimeDataVolumeRow } from '@/lib/runtime-data-volumes'
import { RuntimeDataVolumesEditor } from './application-deployment-data-volumes-editor'

function renderEditor() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onChange = vi.fn()
  const row = { ...emptyRuntimeDataVolumeRow(0), mountPath: '/cache', sourceType: 'emptyDir' as const }
  const result = render(
    <MemoryRouter initialEntries={['/projects/project-1/applications/app-1']}>
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <Routes>
            <Route
              path="/projects/:projectId/applications/:applicationId"
              element={<RuntimeDataVolumesEditor clusterId="cluster-1" enabled onChange={onChange} rows={[row]} runtimeClusters={[]} />}
            />
          </Routes>
        </TooltipProvider>
      </QueryClientProvider>
    </MemoryRouter>,
  )
  return { ...result, onChange, row }
}

describe('deployment data volumes editor', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en-US')
  })

  it('uses a responsive labeled field layout and keeps row actions inside the editor', async () => {
    const user = userEvent.setup()
    const { container, onChange, row } = renderEditor()

    expect(screen.getByLabelText('Name')).toBeInTheDocument()
    expect(screen.getByLabelText('Source')).toBeInTheDocument()
    expect(screen.getByLabelText('Data path')).toBeInTheDocument()
    expect(screen.getByText('Source detail')).toBeInTheDocument()
    expect(container.querySelector('.sm\\:grid-cols-2')).toBeInTheDocument()
    expect(container.innerHTML).not.toContain('minmax(7rem')

    await user.click(screen.getByRole('button', { name: 'Remove data volume' }))
    expect(onChange).toHaveBeenCalledWith([])

    await user.click(screen.getByRole('button', { name: 'Add data volume' }))
    expect(onChange).toHaveBeenLastCalledWith([row, expect.objectContaining({ name: 'data-2' })])
  })
})
