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

  return (
    <div
      aria-label={t('common.cliCommand.label')}
      className={cn('flex min-w-0 items-center gap-2 rounded-control bg-zinc-950 px-2 py-1.5 text-zinc-100', className)}
      role="group"
    >
      <SquareTerminal aria-hidden="true" className="size-3.5 shrink-0 text-emerald-400" />
      <code className="min-w-0 flex-1 overflow-x-auto whitespace-nowrap font-mono text-xs leading-5 text-zinc-300">
        {command}
      </code>
      <Button
        aria-label={copied ? t('common.cliCommand.copied') : t('common.cliCommand.copy')}
        className="size-7 shrink-0 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100 focus-visible:ring-emerald-400/70"
        size="icon"
        title={copied ? t('common.cliCommand.copied') : t('common.cliCommand.copy')}
        type="button"
        variant="ghost"
        onClick={() => void copyCommand()}
      >
        {copied ? <Check aria-hidden="true" className="size-3.5 text-emerald-400" /> : <Copy aria-hidden="true" className="size-3.5" />}
      </Button>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            asChild
            className="size-7 shrink-0 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100 focus-visible:ring-emerald-400/70"
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
