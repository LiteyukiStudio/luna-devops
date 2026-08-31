import type { DeploymentTargetPayload } from '@/api'
import { render } from '@testing-library/react'
import { useForm } from 'react-hook-form'
import { describe, expect, it } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { deploymentTargetDefaults } from '@/pages/applications/deployments/application-deployments-panel-utils'
import { RuntimeResourceFields } from './application-deployment-resource-fields'
import '@/i18n'

function RuntimeResourceFieldsHarness() {
  const form = useForm<DeploymentTargetPayload>({ defaultValues: deploymentTargetDefaults })
  return <RuntimeResourceFields form={form} priceText="1" />
}

describe('runtime resource fields layout', () => {
  it('fills its parent without creating implicit grid columns', () => {
    const { container } = render(
      <TooltipProvider>
        <RuntimeResourceFieldsHarness />
      </TooltipProvider>,
    )
    const root = container.firstElementChild

    expect(root).toHaveClass('w-full')
    expect(root).not.toHaveClass('md:col-span-2')
    expect(root?.firstElementChild).toHaveClass('md:grid-cols-3')
  })
})
