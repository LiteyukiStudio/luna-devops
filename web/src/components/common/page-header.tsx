import type { ReactNode } from 'react'
import { PageChromeTools } from '@/components/common/page-chrome'

/**
 * 页面级操作栏。
 * 页面标题由布局负责，actions 会提升到桌面端标题行；中小屏操作保留在正文流中。
 */
export function PageHeader({ title, description, actions }: { title: string, description?: string, actions?: ReactNode }) {
  void title
  void description

  return <PageChromeTools className="gap-3">{actions}</PageChromeTools>
}
