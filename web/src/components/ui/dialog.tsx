import type { ComponentProps } from 'react'
import { XIcon } from 'lucide-react'
import { Dialog as DialogPrimitive } from 'radix-ui'

import i18next from '@/i18n'
import { cn } from '@/lib/utils'

function Dialog({ ...props }: ComponentProps<typeof DialogPrimitive.Root>) {
  return <DialogPrimitive.Root data-slot="dialog" {...props} />
}

function DialogTrigger({ ...props }: ComponentProps<typeof DialogPrimitive.Trigger>) {
  return <DialogPrimitive.Trigger data-slot="dialog-trigger" {...props} />
}

function DialogPortal({ ...props }: ComponentProps<typeof DialogPrimitive.Portal>) {
  return <DialogPrimitive.Portal data-slot="dialog-portal" {...props} />
}

function DialogClose({ ...props }: ComponentProps<typeof DialogPrimitive.Close>) {
  return <DialogPrimitive.Close data-slot="dialog-close" {...props} />
}

function DialogOverlay({ className, ...props }: ComponentProps<typeof DialogPrimitive.Overlay>) {
  return (
    <DialogPrimitive.Overlay
      className={cn('fixed inset-0 z-50 bg-black/40 data-[state=closed]:animate-out data-[state=open]:animate-in data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0', className)}
      data-slot="dialog-overlay"
      {...props}
    />
  )
}

/**
 * 通用弹窗容器。
 * `scrollable` 开启“固定 Header + 可滚动 DialogBody + 固定 Footer”长表单结构，避免移动端滚到底才能点击主操作。
 */
function DialogContent({
  className,
  children,
  scrollable = false,
  showCloseButton = true,
  ...props
}: ComponentProps<typeof DialogPrimitive.Content> & {
  scrollable?: boolean
  showCloseButton?: boolean
}) {
  return (
    <DialogPortal>
      <DialogOverlay />
      <DialogPrimitive.Content
        className={cn(
          'fixed left-1/2 top-1/2 z-50 max-h-[calc(100dvh-2rem)] w-[calc(100%-2rem)] max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-feature border border-border bg-surface shadow-overlay duration-200 data-[state=closed]:animate-out data-[state=open]:animate-in data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95',
          scrollable ? 'flex flex-col gap-0 overflow-hidden p-0' : 'grid gap-4 overflow-y-auto p-5',
          className,
        )}
        data-scrollable={scrollable || undefined}
        data-slot="dialog-content"
        {...props}
      >
        {children}
        {showCloseButton && (
          <DialogPrimitive.Close
            className={cn(
              'absolute right-4 grid size-9 place-items-center rounded-md text-muted-foreground opacity-80 outline-none transition hover:bg-muted hover:text-foreground hover:opacity-100 focus:ring-2 focus:ring-ring/50 disabled:pointer-events-none',
              scrollable ? 'top-3.5' : 'top-4',
            )}
            data-slot="dialog-close"
          >
            <XIcon className="size-4" />
            <span className="sr-only">{i18next.t('common.close')}</span>
          </DialogPrimitive.Close>
        )}
      </DialogPrimitive.Content>
    </DialogPortal>
  )
}

function DialogHeader({ className, ...props }: ComponentProps<'div'>) {
  return (
    <div
      className={cn(
        'grid gap-1.5 pr-8 text-left [[data-scrollable=true]_&]:shrink-0 [[data-scrollable=true]_&]:border-b [[data-scrollable=true]_&]:border-border [[data-scrollable=true]_&]:px-6 [[data-scrollable=true]_&]:py-4 [[data-scrollable=true]_&]:pr-14',
        className,
      )}
      data-slot="dialog-header"
      {...props}
    />
  )
}

function DialogBody({ className, ...props }: ComponentProps<'div'>) {
  return (
    <div
      className={cn('min-h-0 flex-1 overflow-y-auto px-6 py-4', className)}
      data-slot="dialog-body"
      {...props}
    />
  )
}

function DialogFooter({ className, ...props }: ComponentProps<'div'>) {
  return (
    <div
      className={cn(
        'flex flex-col-reverse gap-2 sm:flex-row sm:justify-end [[data-scrollable=true]_&]:shrink-0 [[data-scrollable=true]_&]:border-t [[data-scrollable=true]_&]:border-border [[data-scrollable=true]_&]:bg-surface [[data-scrollable=true]_&]:px-6 [[data-scrollable=true]_&]:py-3.5',
        className,
      )}
      data-slot="dialog-footer"
      {...props}
    />
  )
}

function DialogTitle({ className, ...props }: ComponentProps<typeof DialogPrimitive.Title>) {
  return (
    <DialogPrimitive.Title
      className={cn('text-lg font-semibold leading-none tracking-normal', className)}
      data-slot="dialog-title"
      {...props}
    />
  )
}

function DialogDescription({ className, ...props }: ComponentProps<typeof DialogPrimitive.Description>) {
  return (
    <DialogPrimitive.Description
      className={cn('text-sm text-muted-foreground', className)}
      data-slot="dialog-description"
      {...props}
    />
  )
}

export {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogPortal,
  DialogTitle,
  DialogTrigger,
}
