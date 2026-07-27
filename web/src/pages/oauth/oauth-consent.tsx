import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { usePublicConfig } from '@/app/public-config-context'
import { PageMotion } from '@/components/common/motion'
import { Card } from '@/components/ui/card'

export function OAuthConsentShell({ children, title }: { children: ReactNode, title: string }) {
  const { t } = useTranslation()
  const configs = usePublicConfig()

  return (
    <div className="grid min-h-screen place-items-center bg-primary-subtle p-4 text-foreground">
      <PageMotion className="w-full max-w-xl">
        <Card className="grid gap-5 p-6 sm:p-8">
          <div className="flex items-center gap-3 border-b border-border pb-5">
            <img alt="" className="size-11 rounded-lg object-contain" src={configs['site.logoUrl'] || '/luna-devops-logo.svg'} />
            <div className="min-w-0">
              <p className="truncate text-sm text-muted-foreground">{configs['site.title'] || t('appName')}</p>
              <h1 className="text-xl font-semibold">{title}</h1>
            </div>
          </div>
          {children}
        </Card>
      </PageMotion>
    </div>
  )
}
