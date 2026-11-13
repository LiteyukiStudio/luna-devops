import { ArrowUpRight, BookOpen, CircleHelp, GitFork } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { documentationHomeUrl } from '@/lib/documentation-url'

const GITHUB_REPOSITORY_URL = 'https://github.com/LiteyukiStudio/luna-devops'

/**
 * 顶栏右上角的帮助与外部资源入口：集中承载公开文档与 GitHub 仓库链接。
 */
export function HelpMenu() {
  const { i18n, t } = useTranslation()
  const docsUrl = documentationHomeUrl(i18n.resolvedLanguage ?? i18n.language)

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          aria-label={t('helpMenu')}
          className="size-8 shrink-0 data-[state=open]:bg-muted"
          size="icon"
          title={t('helpMenu')}
          variant="outline"
        >
          <CircleHelp aria-hidden="true" className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-48" sideOffset={8}>
        <DropdownMenuItem asChild>
          <a href={docsUrl} rel="noreferrer" target="_blank">
            <BookOpen />
            <span className="flex-1">{t('documentation')}</span>
            <ArrowUpRight aria-hidden="true" className="size-3.5 text-muted-foreground" />
          </a>
        </DropdownMenuItem>
        <DropdownMenuItem asChild>
          <a href={GITHUB_REPOSITORY_URL} rel="noreferrer" target="_blank">
            <GitFork />
            <span className="flex-1">{t('githubRepository')}</span>
            <ArrowUpRight aria-hidden="true" className="size-3.5 text-muted-foreground" />
          </a>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
