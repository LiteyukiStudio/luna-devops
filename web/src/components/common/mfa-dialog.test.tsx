import { act, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
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
  })

  it('renders a credential-associated one-time-code form', async () => {
    render(
      <TooltipProvider>
        <MFADialogProvider><main /></MFADialogProvider>
      </TooltipProvider>,
    )

    await waitFor(() => expect(mocks.challengeHandler).toBeTypeOf('function'))
    act(() => {
      void mocks.challengeHandler?.({ purpose: 'password_update' })
    })

    const oneTimeCode = await screen.findByRole('textbox', { name: i18next.t('accountPage.mfa.otpPlaceholder') })
    const form = oneTimeCode.closest('form')
    expect(oneTimeCode).toHaveAttribute('autocomplete', 'one-time-code')
    expect(oneTimeCode).toHaveAttribute('name', 'one-time-code')
    expect(form?.querySelector('input[autocomplete="username"]')).toHaveValue('login@example.test')
    expect(screen.getByRole('button', { name: i18next.t('accountPage.mfa.verify') })).toHaveAttribute('type', 'submit')
  })
})
