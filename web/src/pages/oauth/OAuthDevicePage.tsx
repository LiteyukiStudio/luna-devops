import { useMutation, useQuery } from '@tanstack/react-query'
import { CheckCircle2, KeyRound, ShieldCheck, ShieldX } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useLocation, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { ErrorState } from '@/components/common/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { OAuthConsentShell } from './oauth-consent'
import { oauthScopeLabel, splitOAuthScopes } from './oauth-utils'

type DeviceDecision = 'approved' | 'denied'

export function OAuthDevicePage() {
  const { i18n, t } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate()
  const session = useSession()
  const queryCode = normalizeDeviceCode(new URLSearchParams(location.search).get('user_code') ?? '')
  const [code, setCode] = useState(queryCode)
  const [submittedCode, setSubmittedCode] = useState(queryCode)
  const [completedDecision, setCompletedDecision] = useState<DeviceDecision>()
  const verification = useQuery({
    queryKey: ['oauth-device-verification', submittedCode],
    queryFn: () => api.getOAuthDeviceVerification(submittedCode),
    enabled: Boolean(session.user && submittedCode && !completedDecision),
    retry: false,
  })
  const decision = useMutation({
    mutationFn: (approved: boolean) => api.decideOAuthDeviceVerification({
      approved,
      userCode: verification.data?.userCode || submittedCode,
    }),
    onSuccess: (_result, approved) => setCompletedDecision(approved ? 'approved' : 'denied'),
    onError: error => toast.error(error.message),
  })

  useEffect(() => {
    if (session.initialized && !session.user) {
      const returnTo = `${location.pathname}${location.search}`
      navigate(`/login?redirect=${encodeURIComponent(returnTo)}`, { replace: true })
    }
  }, [location.pathname, location.search, navigate, session.initialized, session.user])

  if (!session.initialized || !session.user)
    return <div className="min-h-screen bg-primary-subtle" />

  const submitCode = () => {
    const normalized = normalizeDeviceCode(code)
    if (!normalized)
      return
    setCode(normalized)
    setSubmittedCode(normalized)
    setCompletedDecision(undefined)
  }

  const resetCode = () => {
    setCode('')
    setSubmittedCode('')
    setCompletedDecision(undefined)
  }

  return (
    <OAuthConsentShell title={t('oauthApps.device.title')}>
      {completedDecision
        ? (
            <DeviceDecisionResult decision={completedDecision} onReset={resetCode} />
          )
        : !submittedCode
            ? (
                <form
                  className="grid gap-5"
                  onSubmit={(event) => {
                    event.preventDefault()
                    submitCode()
                  }}
                >
                  <div className="grid gap-2">
                    <Label htmlFor="oauth-device-code">{t('oauthApps.device.codeLabel')}</Label>
                    <Input
                      id="oauth-device-code"
                      autoCapitalize="characters"
                      autoComplete="off"
                      autoFocus
                      className="font-mono uppercase tracking-wider"
                      placeholder={t('oauthApps.device.codePlaceholder')}
                      spellCheck={false}
                      value={code}
                      onChange={event => setCode(event.target.value.toUpperCase())}
                    />
                    <p className="text-sm text-muted-foreground">{t('oauthApps.device.codeHint')}</p>
                  </div>
                  <div className="flex justify-end">
                    <Button disabled={!code.trim()} type="submit">
                      {t('oauthApps.device.continue')}
                    </Button>
                  </div>
                </form>
              )
            : verification.isLoading
              ? <DeviceVerificationSkeleton />
              : verification.isError
                ? (
                    <div className="grid gap-4">
                      <ErrorState title={t('oauthApps.device.requestInvalid')} description={verification.error.message} />
                      <div className="flex justify-end">
                        <Button variant="secondary" onClick={resetCode}>{t('oauthApps.device.useAnotherCode')}</Button>
                      </div>
                    </div>
                  )
                : verification.data && (
                  <>
                    <div className="flex items-start gap-3">
                      {verification.data.application.logoUrl
                        ? <img alt={verification.data.application.name} className="size-12 rounded-md border border-border object-contain" src={verification.data.application.logoUrl} />
                        : <div className="grid size-12 place-items-center rounded-md bg-primary/10 text-primary"><KeyRound size={22} /></div>}
                      <div className="min-w-0">
                        <h2 className="font-semibold">{verification.data.application.name}</h2>
                        <p className="text-sm text-muted-foreground">{verification.data.application.description || t('oauthApps.noDescription')}</p>
                      </div>
                    </div>
                    <div className="grid gap-3 rounded-md border border-border bg-muted/30 p-4">
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <p className="text-sm font-medium">
                          {t('oauthApps.device.requestsAccess', { application: verification.data.application.name })}
                        </p>
                        <Badge className="font-mono tracking-wide" variant="outline">{verification.data.userCode}</Badge>
                      </div>
                      <div className="flex flex-wrap gap-2">
                        {splitOAuthScopes(verification.data.scope).map(scope => (
                          <Badge key={scope} variant="secondary">{oauthScopeLabel(t, scope)}</Badge>
                        ))}
                      </div>
                      <p className="text-sm text-muted-foreground">
                        {t('oauthApps.device.expiresAt', { time: formatDateTime(verification.data.expiresAt, i18n.language) })}
                      </p>
                    </div>
                    <p className="text-sm text-muted-foreground">{t('oauthApps.revokeAnytime')}</p>
                    <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
                      <Button disabled={decision.isPending} variant="secondary" onClick={() => decision.mutate(false)}>
                        <ShieldX size={16} />
                        {t('oauthApps.device.deny')}
                      </Button>
                      <Button disabled={decision.isPending} onClick={() => decision.mutate(true)}>
                        <ShieldCheck size={16} />
                        {t('oauthApps.device.approve')}
                      </Button>
                    </div>
                  </>
                )}
    </OAuthConsentShell>
  )
}

function DeviceDecisionResult({ decision, onReset }: { decision: DeviceDecision, onReset: () => void }) {
  const { t } = useTranslation()
  const approved = decision === 'approved'
  return (
    <div className="grid justify-items-center gap-4 py-4 text-center">
      <div className={approved ? 'text-success' : 'text-muted-foreground'}>
        {approved ? <CheckCircle2 size={44} /> : <ShieldX size={44} />}
      </div>
      <div className="grid gap-1">
        <h2 className="text-lg font-semibold">
          {t(approved ? 'oauthApps.device.approvedTitle' : 'oauthApps.device.deniedTitle')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t(approved ? 'oauthApps.device.approvedDescription' : 'oauthApps.device.deniedDescription')}
        </p>
      </div>
      <Button variant="secondary" onClick={onReset}>{t('oauthApps.device.useAnotherCode')}</Button>
    </div>
  )
}

function DeviceVerificationSkeleton() {
  return (
    <div className="grid animate-pulse gap-4">
      <div className="h-12 rounded-md bg-muted" />
      <div className="h-32 rounded-md bg-muted" />
      <div className="ml-auto h-9 w-36 rounded-md bg-muted" />
    </div>
  )
}

function normalizeDeviceCode(value: string) {
  return value.trim().toUpperCase()
}

function formatDateTime(value: string, locale: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime())
    ? value
    : new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(date)
}
