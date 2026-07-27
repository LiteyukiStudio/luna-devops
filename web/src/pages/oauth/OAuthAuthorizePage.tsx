import { useMutation, useQuery } from '@tanstack/react-query'
import { KeyRound, ShieldCheck, ShieldX } from 'lucide-react'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { ErrorState } from '@/components/common/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { OAuthConsentShell } from './oauth-consent'
import { oauthScopeLabel, splitOAuthScopes } from './oauth-utils'

export function OAuthAuthorizePage() {
  const { t } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate()
  const session = useSession()
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
    <OAuthConsentShell title={t('oauthApps.authorizeTitle')}>
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
                {splitOAuthScopes(request.data.scope).map(scope => <Badge key={scope} variant="secondary">{oauthScopeLabel(t, scope)}</Badge>)}
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
    </OAuthConsentShell>
  )
}
