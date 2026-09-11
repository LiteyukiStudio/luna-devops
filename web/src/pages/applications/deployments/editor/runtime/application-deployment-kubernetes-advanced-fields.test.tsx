import type { DeploymentTargetPayload } from '@/api'
import { render } from '@testing-library/react'
import { useForm } from 'react-hook-form'
import { describe, expect, it } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { deploymentTargetDefaults } from '@/pages/applications/deployments/application-deployments-panel-utils'
import { KubernetesAdvancedFields } from './application-deployment-kubernetes-advanced-fields'
import '@/i18n'

function KubernetesAdvancedFieldsHarness() {
  const form = useForm<DeploymentTargetPayload>({ defaultValues: deploymentTargetDefaults })
  return <KubernetesAdvancedFields form={form} />
}

describe('kubernetes advanced fields', () => {
  it('requires at least one HPA minimum replica', () => {
    const { container } = render(
      <TooltipProvider>
        <KubernetesAdvancedFieldsHarness />
      </TooltipProvider>,
    )

    expect(container.querySelector('input[name="autoScalingMinReplicas"]')).toHaveAttribute('min', '1')
  })
})
