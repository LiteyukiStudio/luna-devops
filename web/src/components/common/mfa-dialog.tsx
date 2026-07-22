import type { ReactNode } from 'react'
import type { ApiError, MFAPurpose } from '@/api'
import { KeyRound, ShieldCheck } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { registerMFAChallengeHandler } from '@/api/core'
import { OneTimeCodeInput } from '@/components/common/one-time-code-input'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

interface PendingChallenge {
  purpose: MFAPurpose
  reject: (reason?: unknown) => void
  resolve: () => void
}

export function MFADialogProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
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

          <div className="grid gap-3">
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
                    aria-label={t('accountPage.mfa.otpPlaceholder')}
                    autoFocus
                    disabled={verifying}
                    invalid={Boolean(error)}
                    name="one-time-code"
                    value={code}
                    onChange={setCode}
                    onComplete={value => void verify(value)}
                  />
                )
              : (
                  <Input
                    aria-invalid={Boolean(error)}
                    autoComplete="off"
                    name="recovery-code"
                    placeholder={t('accountPage.mfa.recoveryPlaceholder')}
                    type="text"
                    value={code}
                    onChange={event => setCode(event.target.value)}
                    onKeyDown={event => event.key === 'Enter' && void verify()}
                  />
                )}
            {error && <p className="text-sm text-danger">{error}</p>}
          </div>

          <DialogFooter>
            <Button disabled={verifying} variant="secondary" onClick={cancel}>
              {t('cancel')}
            </Button>
            <Button disabled={verifying || code.trim().length < 6} onClick={() => void verify()}>
              <KeyRound size={16} />
              {verifying ? t('accountPage.mfa.verifying') : t('accountPage.mfa.verify')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
