import type { ReactNode } from 'react'
import { AlertTriangle } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'

interface ConfirmDialogProps {
  children?: ReactNode
  content?: ReactNode
  cancelText?: string
  cancelVariant?: 'default' | 'destructive' | 'ghost' | 'link' | 'outline' | 'secondary'
  confirmVariant?: 'default' | 'destructive' | 'ghost' | 'link' | 'outline' | 'secondary'
  closeOnConfirm?: boolean
  open?: boolean
  title: string
  description: string
  confirmText?: string
  pending?: boolean
  onConfirm: () => null | void | Promise<null | void>
  onOpenChange?: (open: boolean) => void
}

/**
 * 危险或不可逆操作的二次确认弹窗。
 * 用于删除、解绑、禁用、权限变更等需要用户明确确认的动作；普通保存、筛选和导航不要使用它。
 */
export function ConfirmDialog({
  children,
  content,
  cancelText,
  cancelVariant = 'secondary',
  confirmVariant = 'destructive',
  closeOnConfirm = true,
  open,
  title,
  description,
  confirmText,
  pending = false,
  onConfirm,
  onOpenChange,
}: ConfirmDialogProps) {
  const { t } = useTranslation()
  const [internalOpen, setInternalOpen] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const isControlled = open !== undefined
  const resolvedOpen = isControlled ? open : internalOpen

  const setOpen = (nextOpen: boolean) => {
    if (!isControlled)
      setInternalOpen(nextOpen)
    onOpenChange?.(nextOpen)
  }

  const handleConfirm = async () => {
    try {
      setConfirming(true)
      await onConfirm()
      if (closeOnConfirm)
        setOpen(false)
    }
    finally {
      setConfirming(false)
    }
  }

  const busy = pending || confirming

  return (
    <Dialog open={resolvedOpen} onOpenChange={setOpen}>
      {children && <DialogTrigger asChild>{children}</DialogTrigger>}
      <DialogContent>
        <div className="flex gap-3">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted text-danger">
            <AlertTriangle size={18} />
          </span>
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>{description}</DialogDescription>
          </DialogHeader>
        </div>
        {content}
        <DialogFooter>
          <DialogClose asChild>
            <Button disabled={busy} variant={cancelVariant}>
              {cancelText ?? t('cancel')}
            </Button>
          </DialogClose>
          <Button disabled={busy} variant={confirmVariant} onClick={handleConfirm}>
            {confirmText ?? t('common.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
