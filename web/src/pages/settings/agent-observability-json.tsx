import type { ReactNode } from 'react'
import { CopyableCodeBlock } from '@/components/common/ai-assistant/copyable-code-block'
import { cn } from '@/lib/utils'
import { formatObservabilityJSON, jsonSyntaxTokens, jsonTokenClassName } from './agent-observability-json-utils'

export function ObservabilityJsonBlock({ className, value }: { className?: string, value: unknown }) {
  const formatted = formatObservabilityJSON(value)
  return (
    <CopyableCodeBlock className={cn('leading-5', className)} value={formatted}>
      <code>
        {jsonSyntaxTokens(value).map(token => (
          <span key={token.id} className={jsonTokenClassName(token.kind)}>{token.value}</span>
        ))}
      </code>
    </CopyableCodeBlock>
  )
}

export function ObservabilityJsonValue({ children }: { children: ReactNode }) {
  return <span className="font-mono text-xs [overflow-wrap:anywhere]">{children}</span>
}
