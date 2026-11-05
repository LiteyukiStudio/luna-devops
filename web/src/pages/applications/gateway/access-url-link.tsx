import { ExternalLink } from 'lucide-react'
import { cn } from '@/lib/utils'
import { displayAccessUrl } from './gateway-route-url'

/** 访问地址外链：等宽、可截断、新窗口打开并隔离 opener。url 需先经 gatewayRouteAccessUrl 校验协议。 */
export function AccessUrlLink({ className, url }: { className?: string, url: string }) {
  return (
    <a
      className={cn('inline-flex min-w-0 max-w-full items-center gap-1 rounded-sm font-mono text-xs text-primary-text hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring', className)}
      href={url}
      rel="noopener noreferrer"
      target="_blank"
      title={url}
    >
      <span className="min-w-0 truncate">{displayAccessUrl(url)}</span>
      <ExternalLink aria-hidden="true" className="size-3 shrink-0" />
    </a>
  )
}
