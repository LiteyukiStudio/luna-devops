import { ArrowRight, Copy, ExternalLink, KeyRound, ShieldCheck, Sparkles, Terminal } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Section } from '@/components/common/section'
import { Surface } from '@/components/common/surface'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { mcpDocumentationUrl } from '@/lib/documentation-url'
import {
  buildClaudeMcpCliSnippet,
  buildMcpAgentPrompt,
  buildMcpServersJsonSnippet,
  resolveMcpEndpointUrl,
} from './account-mcp-helpers'

export function AccountMcpPanel({ onNavigateTab }: {
  onNavigateTab: (tab: 'tokens' | 'oauth-applications') => void
}) {
  const { i18n, t } = useTranslation()
  const endpointUrl = resolveMcpEndpointUrl()
  const docsUrl = mcpDocumentationUrl(i18n.language)
  const aiPrompt = buildMcpAgentPrompt(t, { endpointUrl })
  const jsonSnippet = buildMcpServersJsonSnippet(endpointUrl)
  const cliSnippet = buildClaudeMcpCliSnippet(endpointUrl)

  const copyText = (value: string, successMessage = t('common.copied')) => {
    navigator.clipboard.writeText(value)
      .then(() => toast.success(successMessage))
      .catch((error: unknown) => toast.error(error instanceof Error ? error.message : String(error)))
  }

  return (
    <div className="grid gap-4">
      <Section
        description={t('accountPage.mcp.endpointDescription')}
        title={t('accountPage.mcp.endpointTitle')}
        tools={(
          <Button asChild size="sm" variant="outline">
            <a href={docsUrl} rel="noopener noreferrer" target="_blank">
              {t('accountPage.mcp.docsLink')}
              <ExternalLink className="size-3.5" />
            </a>
          </Button>
        )}
        variant="bordered"
      >
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <Badge variant="secondary">{t('accountPage.mcp.transportBadge')}</Badge>
          <div className="flex min-w-0 flex-1 items-center gap-2">
            <code
              className="min-w-0 flex-1 truncate rounded-control border border-border bg-surface-inset px-3 py-1.5 font-mono text-xs text-foreground"
              data-testid="mcp-endpoint-url"
            >
              {endpointUrl}
            </code>
            <Button
              aria-label={t('accountPage.mcp.copyEndpoint')}
              size="sm"
              type="button"
              variant="outline"
              onClick={() => copyText(endpointUrl)}
            >
              <Copy className="size-3.5" />
              {t('accountPage.mcp.copyEndpoint')}
            </Button>
          </div>
        </div>
        <p className="text-xs leading-relaxed text-muted-foreground">
          {t('accountPage.mcp.metaToolsHint')}
        </p>
      </Section>

      <Section
        description={t('accountPage.mcp.promptDescription')}
        icon={<Sparkles className="size-4" />}
        title={t('accountPage.mcp.promptTitle')}
        tools={(
          <Button
            size="sm"
            type="button"
            onClick={() => copyText(aiPrompt, t('accountPage.mcp.promptCopied'))}
          >
            <Copy className="size-3.5" />
            {t('accountPage.mcp.copyPrompt')}
          </Button>
        )}
        variant="bordered"
      >
        <Surface className="p-3.5" variant="inset">
          <pre
            className="overflow-x-auto font-mono text-xs leading-relaxed whitespace-pre-wrap break-words text-foreground"
            data-testid="mcp-agent-prompt"
          >
            {aiPrompt}
          </pre>
        </Surface>
      </Section>

      <Section
        description={t('accountPage.mcp.authAndManualDescription')}
        icon={<Terminal className="size-4" />}
        title={t('accountPage.mcp.authAndManualTitle')}
        variant="bordered"
      >
        <div className="grid gap-3 sm:grid-cols-2">
          <Surface className="flex flex-col justify-between gap-3 p-4" variant="inset">
            <div className="grid gap-1.5">
              <div className="flex items-center gap-2">
                <KeyRound className="size-4 text-muted-foreground" />
                <h3 className="text-sm font-medium text-foreground">{t('accountPage.mcp.patTitle')}</h3>
              </div>
              <p className="text-xs leading-relaxed text-muted-foreground">
                {t('accountPage.mcp.patDescription')}
              </p>
            </div>
            <div>
              <Button size="sm" type="button" variant="outline" onClick={() => onNavigateTab('tokens')}>
                {t('accountPage.mcp.goToTokens')}
                <ArrowRight className="size-3.5" />
              </Button>
            </div>
          </Surface>

          <Surface className="flex flex-col justify-between gap-3 p-4" variant="inset">
            <div className="grid gap-1.5">
              <div className="flex items-center gap-2">
                <ShieldCheck className="size-4 text-muted-foreground" />
                <h3 className="text-sm font-medium text-foreground">{t('accountPage.mcp.oauthTitle')}</h3>
              </div>
              <p className="text-xs leading-relaxed text-muted-foreground">
                {t('accountPage.mcp.oauthDescription')}
              </p>
            </div>
            <div>
              <Button size="sm" type="button" variant="outline" onClick={() => onNavigateTab('oauth-applications')}>
                {t('accountPage.mcp.goToOAuthApps')}
                <ArrowRight className="size-3.5" />
              </Button>
            </div>
          </Surface>
        </div>

        <div className="grid gap-3">
          <div className="grid gap-1.5">
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs font-medium text-muted-foreground">
                {t('accountPage.mcp.jsonConfigLabel')}
              </span>
              <Button
                aria-label={t('accountPage.mcp.copyJsonConfig')}
                size="sm"
                type="button"
                variant="ghost"
                onClick={() => copyText(jsonSnippet)}
              >
                <Copy className="size-3.5" />
                {t('accountPage.mcp.copyJsonConfig')}
              </Button>
            </div>
            <Surface className="p-3.5" variant="inset">
              <pre className="overflow-x-auto font-mono text-xs leading-relaxed text-foreground">
                {jsonSnippet}
              </pre>
            </Surface>
          </div>

          <div className="grid gap-1.5">
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs font-medium text-muted-foreground">
                {t('accountPage.mcp.cliConfigLabel')}
              </span>
              <Button
                aria-label={t('accountPage.mcp.copyCliConfig')}
                size="sm"
                type="button"
                variant="ghost"
                onClick={() => copyText(cliSnippet)}
              >
                <Copy className="size-3.5" />
                {t('accountPage.mcp.copyCliConfig')}
              </Button>
            </div>
            <Surface className="p-3.5" variant="inset">
              <pre className="overflow-x-auto font-mono text-xs leading-relaxed text-foreground">
                {cliSnippet}
              </pre>
            </Surface>
          </div>
        </div>
      </Section>
    </div>
  )
}
