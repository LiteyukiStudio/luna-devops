const filenameUnsafePattern = /[^\w.-]+/g

export function sanitizeKubeconfigFilenamePart(value: string) {
  return value
    .trim()
    .replace(filenameUnsafePattern, '-')
    .replace(/-+/g, '-')
    .replace(/^[-_.]+|[-_.]+$/g, '')
    .toLowerCase()
}

export function kubeconfigFilename(name: string) {
  const sanitized = sanitizeKubeconfigFilenamePart(name)
  return sanitized ? `luna-kubeconfig-${sanitized}.yaml` : 'luna-kubeconfig.yaml'
}

export function downloadKubeconfigFile(name: string, kubeconfig: string) {
  const url = URL.createObjectURL(new Blob([kubeconfig], { type: 'application/yaml;charset=utf-8' }))
  const link = document.createElement('a')
  link.href = url
  link.download = kubeconfigFilename(name)
  link.hidden = true
  document.body.append(link)
  link.click()
  link.remove()
  queueMicrotask(() => URL.revokeObjectURL(url))
}
