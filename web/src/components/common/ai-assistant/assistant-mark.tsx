import { Sparkles } from 'lucide-react'
import { cn } from '@/lib/utils'

/** Luna 助手的小标识：头部、欢迎页与助手回复共用，保持同一视觉锚点。 */
export function AIAssistantMark({ className, size = 'sm' }: { className?: string, size?: 'sm' | 'lg' }) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        'grid shrink-0 place-items-center rounded-full border border-primary-border bg-primary-subtle text-primary-text',
        size === 'lg' ? 'size-11 [&_svg]:size-5' : 'size-6 [&_svg]:size-3.5',
        className,
      )}
    >
      <Sparkles />
    </span>
  )
}
