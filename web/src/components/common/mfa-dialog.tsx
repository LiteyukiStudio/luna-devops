import type { ReactNode } from 'react'
import type { ApiError } from '@/api'
import { KeyRound, ShieldCheck } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { registerMFAChallengeHandler } from '@/api/core'
import { useSession } from '@/app/session-context'
import { OneTimeCodeInput } from '@/components/common/one-time-code-input'
import { PasswordManagerUsernameField } from '@/components/common/password-manager-username-field'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

interface PendingChallenge {
  purpose: string
  reject: (reason?: unknown) => void
  resolve: () => void
}

const mfaChallengeErrorId = 'mfa-challenge-error'

export function MFADialogProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const { actualUser, pendingLoginUsername, user } = useSession()
  const [challenge, setChallenge] = useState<PendingChallenge>()
  const [code, setCode] = useState('')
  const [method, setMethod] = useState<'otp' | 'recovery'>('otp')
  const [error, setError] = useState('')
  const [verifying, setVerifying] = useState(false)

  useEffect(() => registerMFAChallengeHandler(({ purpose }) => new Promise<void>((resolve, reject) => {
    setCode('')
    setError('')
    setMethod('otp')
    setChallenge({ purpose, reject, resolve })
  })), [])

  const cancel = () => {
    if (!challenge || verifying)
      return
    challenge.reject(new Error('mfa_challenge_cancelled'))
    setChallenge(undefined)
  }

  const verify = async (candidate = code) => {
    if (!challenge || candidate.trim().length < 6)
      return

    try {
      setVerifying(true)
      setError('')
      const value = candidate.trim()
      await api.verifyMFA(method === 'recovery'
        ? { recoveryCode: value, purpose: challenge.purpose }
        : { code: value, purpose: challenge.purpose })
      challenge.resolve()
      setChallenge(undefined)
    }
    catch (requestError) {
      setError((requestError as ApiError).message || t('accountPage.mfa.verifyFailed'))
    }
    finally {
      setVerifying(false)
    }
  }

  const purposeLabel = challenge
    ? t(`accountPage.mfa.purposes.${challenge.purpose}`, { defaultValue: t('accountPage.mfa.sensitiveOperation') })
    : t('accountPage.mfa.sensitiveOperation')

  return (
    <>
      {children}
      <Dialog open={Boolean(challenge)} onOpenChange={open => !open && cancel()}>
        <DialogContent showCloseButton={!verifying}>
          <div className="flex gap-3">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary-text">
              <ShieldCheck size={18} />
            </span>
            <DialogHeader>
              <DialogTitle>{t('accountPage.mfa.challengeTitle')}</DialogTitle>
              <DialogDescription>{t('accountPage.mfa.challengeDescription', { purpose: purposeLabel })}</DialogDescription>
            </DialogHeader>
          </div>

          <form
            className="grid min-w-0 gap-3"
            onSubmit={(event) => {
              event.preventDefault()
              void verify()
            }}
          >
            <PasswordManagerUsernameField value={pendingLoginUsername ?? (actualUser ?? user)?.email} />
            <Select
              value={method}
              onValueChange={(value) => {
                setMethod(value as 'otp' | 'recovery')
                setCode('')
                setError('')
              }}
            >
              <SelectTrigger aria-label={t('accountPage.mfa.verificationMethod')} className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="otp">{t('accountPage.mfa.otpMethod')}</SelectItem>
                <SelectItem value="recovery">{t('accountPage.mfa.recoveryMethod')}</SelectItem>
              </SelectContent>
            </Select>
            {method === 'otp'
              ? (
                  <OneTimeCodeInput
                    aria-describedby={error ? mfaChallengeErrorId : undefined}
                    aria-label={t('accountPage.mfa.otpPlaceholder')}
                    autoFocus
                    disabled={verifying}
                    invalid={Boolean(error)}
                    value={code}
                    onChange={setCode}
                    onComplete={value => void verify(value)}
                  />
                )
              : (
                  <Input
                    aria-describedby={error ? mfaChallengeErrorId : undefined}
                    aria-invalid={Boolean(error)}
                    autoComplete="off"
                    name="recovery-code"
                    placeholder={t('accountPage.mfa.recoveryPlaceholder')}
                    type="text"
                    value={code}
                    onChange={event => setCode(event.target.value)}
                  />
                )}
            {error && <p id={mfaChallengeErrorId} className="text-sm text-danger" role="alert">{error}</p>}
            <DialogFooter>
              <Button disabled={verifying} type="button" variant="secondary" onClick={cancel}>
                {t('cancel')}
              </Button>
              <Button disabled={verifying || code.trim().length < 6} type="submit">
                <KeyRound size={16} />
                {verifying ? t('accountPage.mfa.verifying') : t('accountPage.mfa.verify')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  )
}
