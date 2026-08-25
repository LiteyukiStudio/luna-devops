import type { AICapabilities } from '@/api'
import { AIAssistantDesktopHost } from './desktop-host'
import { AIAssistantRuntimeProvider } from './runtime-provider'

/**
 * 迁移兼容入口：AppLayout 切换到稳定 Runtime Provider 前继续保留原调用方式。
 */
export function AiAssistant({ capabilities, initiallyOpen = false }: { capabilities: AICapabilities, initiallyOpen?: boolean }) {
  return (
    <AIAssistantRuntimeProvider capabilities={capabilities} initiallyOpen={initiallyOpen}>
      <AIAssistantDesktopHost />
    </AIAssistantRuntimeProvider>
  )
}
