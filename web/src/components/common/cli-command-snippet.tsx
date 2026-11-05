import type { CliInvocationSpec } from '@/lib/cli-command'
import { Check, CircleHelp, Copy, SquareTerminal } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { buildCliCommand } from '@/lib/cli-command'
import { cliDocumentationUrl } from '@/lib/documentation-url'
import { cn } from '@/lib/utils'

const COPY_FEEDBACK_DURATION_MS = 2_000

export function CliCommandSnippet({ className, spec }: { className?: string, spec: CliInvocationSpec }) {
  const { i18n, t } = useTranslation()
  const command = buildCliCommand(spec)
  const [copiedCommand, setCopiedCommand] = useState('')
  const copyResetTimerRef = useRef<number | undefined>(undefined)
  const copied = copiedCommand === command

  useEffect(() => () => window.clearTimeout(copyResetTimerRef.current), [])

  const copyCommand = async () => {
    try {
      await navigator.clipboard.writeText(command)
      window.clearTimeout(copyResetTimerRef.current)
      setCopiedCommand(command)
      copyResetTimerRef.current = window.setTimeout(setCopiedCommand, COPY_FEEDBACK_DURATION_MS, '')
      toast.success(t('common.copied'))
    }
    catch {
      toast.error(t('common.copyFailed'))
    }
  }

  const copyLabel = copied ? t('common.cliCommand.copied') : t('common.cliCommand.copy')
  const actionClassName = 'size-7 shrink-0 text-terminal-muted hover:bg-terminal-foreground/10 hover:text-terminal-foreground'

  return (
    <div
      aria-label={t('common.cliCommand.label')}
      className={cn('flex min-w-0 items-center gap-2 rounded-control border border-terminal-border bg-terminal py-1 pl-2.5 pr-1 text-terminal-foreground', className)}
      role="group"
    >
      <SquareTerminal aria-hidden="true" className="size-3.5 shrink-0 text-terminal-success" />
      <code className="min-w-0 flex-1 overflow-x-auto whitespace-nowrap font-mono text-xs leading-5 selection:bg-terminal-selection">
        {command}
      </code>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            aria-label={copyLabel}
            className={actionClassName}
            size="icon"
            type="button"
            variant="ghost"
            onClick={() => void copyCommand()}
          >
            {copied ? <Check aria-hidden="true" className="size-3.5 text-terminal-success" /> : <Copy aria-hidden="true" className="size-3.5" />}
          </Button>
        </TooltipTrigger>
        <TooltipContent side="top">{copyLabel}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            asChild
            className={actionClassName}
            size="icon"
            variant="ghost"
          >
            <a
              aria-label={t('common.cliCommand.docs')}
              href={cliDocumentationUrl(i18n.resolvedLanguage ?? i18n.language)}
              rel="noreferrer"
              target="_blank"
            >
              <CircleHelp aria-hidden="true" className="size-3.5" />
            </a>
          </Button>
        </TooltipTrigger>
        <TooltipContent side="top">
          {t('common.cliCommand.docsTip')}
        </TooltipContent>
      </Tooltip>
    </div>
  )
}
