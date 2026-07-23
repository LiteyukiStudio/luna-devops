import { useMutation, useQuery } from '@tanstack/react-query'
import { KeyRound, ShieldCheck, ShieldX } from 'lucide-react'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api'
import { usePublicConfig } from '@/app/public-config-context'
import { useSession } from '@/app/session-context'
import { ErrorState } from '@/components/common/error-state'
import { PageMotion } from '@/components/common/motion'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'

export function OAuthAuthorizePage() {
  const { t } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate()
  const session = useSession()
  const configs = usePublicConfig()
  const rawQuery = location.search.slice(1)
  const params = new URLSearchParams(location.search)
  const request = useQuery({
    queryKey: ['oauth-authorization-request', rawQuery],
    queryFn: () => api.getOAuthAuthorizationRequest(rawQuery),
    enabled: Boolean(session.user),
    retry: false,
  })
  const decision = useMutation({
    mutationFn: (approved: boolean) => api.decideOAuthAuthorization({
      approved,
      clientId: params.get('client_id') ?? '',
      redirectUri: params.get('redirect_uri') ?? '',
      scope: params.get('scope') ?? '',
      state: params.get('state') ?? '',
      codeChallenge: params.get('code_challenge') ?? '',
      codeChallengeMethod: params.get('code_challenge_method') ?? '',
    }),
    onSuccess: result => window.location.assign(result.redirectUrl),
    onError: error => toast.error(error.message),
  })

  useEffect(() => {
    if (session.initialized && !session.user) {
      const returnTo = `${location.pathname}${location.search}`
      navigate(`/login?redirect=${encodeURIComponent(returnTo)}`, { replace: true })
    }
  }, [location.pathname, location.search, navigate, session.initialized, session.user])

  if (!session.initialized || !session.user || request.isLoading)
    return <div className="min-h-screen bg-primary-subtle" />

  return (
    <div className="grid min-h-screen place-items-center bg-primary-subtle p-4 text-foreground">
      <PageMotion className="w-full max-w-xl">
        <Card className="grid gap-5 p-6 sm:p-8">
          <div className="flex items-center gap-3 border-b border-border pb-5">
            <img alt="" className="size-11 rounded-lg object-contain" src={configs['site.logoUrl'] || '/luna-devops-logo.svg'} />
            <div className="min-w-0">
              <p className="truncate text-sm text-muted-foreground">{configs['site.title'] || t('appName')}</p>
              <h1 className="text-xl font-semibold">{t('oauthApps.authorizeTitle')}</h1>
            </div>
          </div>

          {request.isError
            ? <ErrorState title={t('oauthApps.authorizationInvalid')} description={request.error.message} />
            : request.data && (
              <>
                <div className="flex items-start gap-3">
                  {request.data.application.logoUrl
                    ? <img alt={request.data.application.name} className="size-12 rounded-md border border-border object-contain" src={request.data.application.logoUrl} />
                    : <div className="grid size-12 place-items-center rounded-md bg-primary/10 text-primary"><KeyRound size={22} /></div>}
                  <div className="min-w-0">
                    <h2 className="font-semibold">{request.data.application.name}</h2>
                    <p className="text-sm text-muted-foreground">{request.data.application.description || t('oauthApps.noDescription')}</p>
                  </div>
                </div>
                <div className="grid gap-3 rounded-md border border-border bg-muted/30 p-4">
                  <p className="text-sm font-medium">{t('oauthApps.requestsAccess')}</p>
                  <div className="flex flex-wrap gap-2">
                    {splitScopes(request.data.scope).map(scope => <Badge key={scope} variant="secondary">{scopeLabel(t, scope)}</Badge>)}
                  </div>
                  <p className="text-sm text-muted-foreground">
                    {request.data.accessTokenLifetimeDays === 0
                      ? t('oauthApps.authorizationNeverExpires')
                      : t('oauthApps.authorizationExpires', { count: request.data.accessTokenLifetimeDays })}
                  </p>
                  {request.data.previouslyAuthorized && <p className="text-sm text-muted-foreground">{t('oauthApps.previouslyAuthorized')}</p>}
                </div>
                <p className="text-sm text-muted-foreground">{t('oauthApps.revokeAnytime')}</p>
                <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
                  <Button disabled={decision.isPending} variant="secondary" onClick={() => decision.mutate(false)}>
                    <ShieldX size={16} />
                    {t('oauthApps.deny')}
                  </Button>
                  <Button disabled={decision.isPending} onClick={() => decision.mutate(true)}>
                    <ShieldCheck size={16} />
                    {t('oauthApps.authorize')}
                  </Button>
                </div>
              </>
            )}
        </Card>
      </PageMotion>
    </div>
  )
}

function splitScopes(value: string) {
  return value.split(/[\s,]+/).filter(Boolean)
}

function scopeLabel(t: ReturnType<typeof useTranslation>['t'], scope: string) {
  const key = `accessTokens.scopeLabels.${scope.replaceAll(':', '.').replaceAll('_', '-')}`
  const translated = t(key)
  return translated === key ? scope : translated
}
