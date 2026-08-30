import type { ServiceBinding } from '@/api'

export function serviceBindingEnvSummary(binding: ServiceBinding) {
  if (binding.injectionMode === 'url')
    return binding.urlEnvVar
  return [binding.hostEnvVar, binding.portEnvVar].filter(Boolean).join(' / ')
}
