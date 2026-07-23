import type { ComponentProps } from 'react'

import { cn } from '@/lib/utils'

function Sidebar({ className, ...props }: ComponentProps<'aside'>) {
  return (
    <aside
      className={cn('sticky top-0 hidden h-screen w-64 min-w-64 max-w-64 shrink-0 flex-col overflow-x-hidden bg-transparent lg:flex', className)}
      data-slot="sidebar"
      {...props}
    />
  )
}

function SidebarHeader({ className, ...props }: ComponentProps<'div'>) {
  return <div className={cn('min-w-0 max-w-full shrink-0 overflow-hidden', className)} data-slot="sidebar-header" {...props} />
}

function SidebarContent({ className, ...props }: ComponentProps<'nav'>) {
  return <nav className={cn('min-h-0 w-full min-w-0 max-w-full flex-1 overscroll-contain overflow-x-hidden overflow-y-auto scroll-pb-4 px-3 py-4', className)} data-slot="sidebar-content" {...props} />
}

function SidebarFooter({ className, ...props }: ComponentProps<'div'>) {
  return <div className={cn('grid w-full min-w-0 max-w-full shrink-0 gap-3 overflow-hidden border-t border-primary-border/30 p-3', className)} data-slot="sidebar-footer" {...props} />
}

function SidebarGroup({ className, ...props }: ComponentProps<'section'>) {
  return <section className={cn('grid w-full min-w-0 max-w-full gap-2 overflow-hidden', className)} data-slot="sidebar-group" {...props} />
}

function SidebarGroupLabel({ className, ...props }: ComponentProps<'p'>) {
  return (
    <p
      className={cn('px-3 text-[0.6875rem] font-normal uppercase tracking-wide text-muted-foreground/80', className)}
      data-slot="sidebar-group-label"
      {...props}
    />
  )
}

function SidebarMenu({ className, ...props }: ComponentProps<'div'>) {
  return <div className={cn('w-full min-w-0 max-w-full space-y-1 overflow-hidden', className)} data-slot="sidebar-menu" {...props} />
}

function SidebarMenuItem({ className, ...props }: ComponentProps<'div'>) {
  return <div className={cn('group/menu-item w-full min-w-0 max-w-full overflow-hidden', className)} data-slot="sidebar-menu-item" {...props} />
}

export {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuItem,
}
