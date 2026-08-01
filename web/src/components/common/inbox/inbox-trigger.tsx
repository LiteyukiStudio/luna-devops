import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Bell } from 'lucide-react'
import { useEffect, useState, useSyncExternalStore } from 'react'
import { useTranslation } from 'react-i18next'
import { api, inboxStreamUrl } from '@/api'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from '@/components/ui/sheet'
import { InboxPanel } from './inbox-panel'
import { inboxKeys, invalidateInbox } from './inbox-query'

const mobileQuery = '(max-width: 47.999rem)'

export function InboxTrigger() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const mobile = useSyncExternalStore(subscribeMobile, mobileSnapshot, () => false)
  const [open, setOpen] = useState(false)
  const unread = useQuery({ queryKey: inboxKeys.unreadCount, queryFn: api.getInboxUnreadCount })
  const count = unread.data?.unreadCount ?? 0

  useEffect(() => {
    if (typeof EventSource === 'undefined')
      return undefined
    const stream = new EventSource(inboxStreamUrl(), { withCredentials: true })
    const refresh = () => void invalidateInbox(queryClient)
    stream.addEventListener('inbox.changed', refresh)
    return () => {
      stream.removeEventListener('inbox.changed', refresh)
      stream.close()
    }
  }, [queryClient])

  const button = (
    <Button aria-label={count > 99 ? t('inbox.unreadCountOverflow') : count > 0 ? t('inbox.unreadCount', { count }) : t('inbox.open')} className="relative" size="icon" variant="ghost">
      <Bell className="size-5" />
      {count > 0 && (
        <span className="absolute -right-1 -top-1 grid h-5 min-w-5 place-items-center rounded-full bg-danger px-1 text-[10px] font-semibold leading-none text-white">
          {count > 99 ? '99+' : count}
        </span>
      )}
    </Button>
  )

  if (mobile) {
    return (
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetTrigger asChild>{button}</SheetTrigger>
        <SheetContent className="flex w-[min(92vw,28rem)] max-w-none gap-0 p-0" side="right">
          <SheetTitle className="sr-only">{t('inbox.title')}</SheetTitle>
          <InboxPanel onClose={() => setOpen(false)} />
        </SheetContent>
      </Sheet>
    )
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{button}</PopoverTrigger>
      <PopoverContent align="end" className="flex h-[min(70vh,38rem)] w-[25rem] max-w-[calc(100vw-2rem)] gap-0 overflow-hidden p-0">
        <InboxPanel onClose={() => setOpen(false)} />
      </PopoverContent>
    </Popover>
  )
}

function subscribeMobile(callback: () => void) {
  const query = window.matchMedia(mobileQuery)
  query.addEventListener('change', callback)
  return () => query.removeEventListener('change', callback)
}

function mobileSnapshot() {
  return window.matchMedia(mobileQuery).matches
}
