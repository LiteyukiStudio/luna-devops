import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import i18next from '@/i18n'
import { MFADialogProvider } from './mfa-dialog'

type ChallengeHandler = (challenge: { purpose: string }) => Promise<void>

const mocks = vi.hoisted(() => ({
  challengeHandler: undefined as ChallengeHandler | undefined,
  verifyMFA: vi.fn(),
}))

vi.mock('@/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      verifyMFA: mocks.verifyMFA,
    },
  }
})

vi.mock('@/api/core', () => ({
  registerMFAChallengeHandler: vi.fn((handler: ChallengeHandler) => {
    mocks.challengeHandler = handler
    return () => {
      mocks.challengeHandler = undefined
    }
  }),
}))

vi.mock('@/app/session-context', () => ({
  useSession: () => ({
    pendingLoginUsername: 'login@example.test',
  }),
}))

describe('mfa challenge dialog', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    mocks.challengeHandler = undefined
    await i18next.changeLanguage('en-US')
    vi.useFakeTimers()
  })

  afterEach(() => {
    cleanup()
    act(() => vi.runOnlyPendingTimers())
    vi.useRealTimers()
  })

  it('renders a credential-associated one-time-code form', () => {
    render(
      <TooltipProvider>
        <MFADialogProvider><main /></MFADialogProvider>
      </TooltipProvider>,
    )

    expect(mocks.challengeHandler).toBeTypeOf('function')
    act(() => {
      void mocks.challengeHandler?.({ purpose: 'password_update' })
    })

    const oneTimeCode = screen.getByRole('textbox', { name: i18next.t('accountPage.mfa.otpPlaceholder') })
    const form = oneTimeCode.closest('form')
    expect(oneTimeCode).toHaveAttribute('autocomplete', 'one-time-code')
    expect(oneTimeCode).toHaveAttribute('name', 'one-time-code')
    expect(form?.querySelector('input[autocomplete="username"]')).toHaveValue('login@example.test')
    expect(screen.getByRole('button', { name: i18next.t('accountPage.mfa.verify') })).toHaveAttribute('type', 'submit')
  })

  it('shows the accepted recovery-code format and explains that hyphens are optional', () => {
    render(
      <TooltipProvider>
        <MFADialogProvider><main /></MFADialogProvider>
      </TooltipProvider>,
    )

    expect(mocks.challengeHandler).toBeTypeOf('function')
    act(() => {
      void mocks.challengeHandler?.({ purpose: 'password_update' })
    })

    screen.getByRole('combobox', { name: i18next.t('accountPage.mfa.verificationMethod') })
    const nativeSelect = document.querySelector('select[aria-hidden="true"]')
    expect(nativeSelect).toBeInstanceOf(HTMLSelectElement)
    fireEvent.change(nativeSelect as HTMLSelectElement, { target: { value: 'recovery' } })

    expect(screen.getByPlaceholderText('XXXX-XXXX-XXXX-XXXX (hyphens optional)')).toHaveAttribute('name', 'recovery-code')
  })

  it('announces a verification failure and associates it with the code input', async () => {
    mocks.verifyMFA.mockRejectedValueOnce(new Error('Invalid verification code'))
    render(
      <TooltipProvider>
        <MFADialogProvider><main /></MFADialogProvider>
      </TooltipProvider>,
    )

    expect(mocks.challengeHandler).toBeTypeOf('function')
    act(() => {
      void mocks.challengeHandler?.({ purpose: 'password_update' })
    })

    const oneTimeCode = screen.getByRole('textbox', { name: i18next.t('accountPage.mfa.otpPlaceholder') })
    await act(async () => {
      fireEvent.change(oneTimeCode, { target: { value: '123456' } })
    })

    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent('Invalid verification code')
    expect(oneTimeCode).toHaveAttribute('aria-describedby', alert.id)
  })

  it('submits a completed TOTP only once when completion and form submit race', async () => {
    let finishVerification: (() => void) | undefined
    mocks.verifyMFA.mockImplementationOnce(() => new Promise<void>((resolve) => {
      finishVerification = resolve
    }))
    render(
      <TooltipProvider>
        <MFADialogProvider><main /></MFADialogProvider>
      </TooltipProvider>,
    )
    act(() => {
      void mocks.challengeHandler?.({ purpose: 'ai_conversation_tools' })
    })
    const oneTimeCode = screen.getByRole('textbox', { name: i18next.t('accountPage.mfa.otpPlaceholder') })
    const form = oneTimeCode.closest('form')!
    fireEvent.change(oneTimeCode, { target: { value: '123456' } })
    fireEvent.submit(form)
    expect(mocks.verifyMFA).toHaveBeenCalledTimes(1)
    await act(async () => {
      finishVerification?.()
      await Promise.resolve()
    })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
})
