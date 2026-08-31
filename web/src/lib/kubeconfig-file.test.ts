import { describe, expect, it, vi } from 'vitest'
import { downloadKubeconfigFile, kubeconfigFilename, sanitizeKubeconfigFilenamePart } from './kubeconfig-file'

describe('kubeconfig file helpers', () => {
  it('sanitizes filenames into stable yaml downloads', () => {
    expect(sanitizeKubeconfigFilenamePart(' Dev Access / Prod ')).toBe('dev-access-prod')
    expect(kubeconfigFilename('')).toBe('luna-kubeconfig.yaml')
    expect(kubeconfigFilename('Dev Access')).toBe('luna-kubeconfig-dev-access.yaml')
  })

  it('downloads a blob and revokes the temporary object URL', async () => {
    const createObjectURL = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:kubeconfig')
    const revokeObjectURL = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})

    downloadKubeconfigFile('Dev Access', 'apiVersion: v1\n')
    await Promise.resolve()

    expect(createObjectURL).toHaveBeenCalledOnce()
    expect(click).toHaveBeenCalledOnce()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:kubeconfig')
  })
})
