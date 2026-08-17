import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'

const updateToastId = 'app-update-available'
const updateCheckIntervalMs = 60_000

interface AppUpdateCheckerProps {
  commitSha?: string
  enabled?: boolean
}

export function AppUpdateChecker({
  commitSha = import.meta.env.VITE_APP_COMMIT_SHA,
  enabled = !import.meta.env.DEV,
}: AppUpdateCheckerProps = {}) {
  const { t } = useTranslation()
  const checkingRef = useRef(false)
  const toastShownRef = useRef(false)
  const mountedRef = useRef(false)
  const normalizedCommitSha = commitSha?.trim() ?? ''

  useEffect(() => {
    if (!enabled || !normalizedCommitSha || normalizedCommitSha === 'dev')
      return

    mountedRef.current = true

    const checkForUpdate = async () => {
      if (checkingRef.current)
        return
      checkingRef.current = true
      try {
        const meta = await api.getAPIMeta()
        const serverVersion = typeof meta.serverVersion === 'string' ? meta.serverVersion.trim() : ''
        if (!mountedRef.current || !serverVersion || serverVersion === normalizedCommitSha || toastShownRef.current)
          return

        toastShownRef.current = true
        toast.info(t('common.siteUpdated'), {
          id: updateToastId,
          duration: Infinity,
          dismissible: false,
          action: {
            label: t('common.refresh'),
            onClick: () => window.location.reload(),
          },
        })
      }
      catch {
        // Version checks are best effort and must not interrupt the current page.
      }
      finally {
        checkingRef.current = false
      }
    }

    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible')
        void checkForUpdate()
    }

    void checkForUpdate()
    const timer = window.setInterval(() => void checkForUpdate(), updateCheckIntervalMs)
    document.addEventListener('visibilitychange', handleVisibilityChange)

    return () => {
      mountedRef.current = false
      window.clearInterval(timer)
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  }, [enabled, normalizedCommitSha, t])

  return null
}
