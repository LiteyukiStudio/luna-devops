import type { AIConversation } from '@/api'
import { ArrowLeft, CheckSquare2, Ellipsis, ListChecks, LoaderCircle, LockKeyhole, MessageSquareText, Pencil, Search, Trash2, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { formatAIConversationTimestamp } from './conversation-timestamp'

export interface AIConversationListProps {
  activeId?: string
  conversations: AIConversation[]
  deleting: boolean
  loading: boolean
  runningConversationIds: Set<string>
  search: string
  variant?: 'drawer' | 'sidebar' | 'mobile'
  onBack?: () => void
  onDeleteMany: (ids: string[]) => Promise<void>
  onRename: (id: string, title: string) => void
  onSearch: (search: string) => void
  onSelect: (id: string) => void
}

export function AIConversationList({
  activeId,
  conversations,
  deleting,
  loading,
  runningConversationIds,
  search,
  variant = 'sidebar',
  onBack,
  onDeleteMany,
  onRename,
  onSearch,
  onSelect,
}: AIConversationListProps) {
  const { t, i18n } = useTranslation()
  const [selecting, setSelecting] = useState(false)
  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(() => new Set())
  const [renamingId, setRenamingId] = useState<string>()
  const [deleteTargets, setDeleteTargets] = useState<AIConversation[]>([])
  const [title, setTitle] = useState('')
  const visibleIds = useMemo(() => conversations.map(conversation => conversation.id), [conversations])
  const selectedIds = visibleIds.filter(id => selectedKeys.has(id))
  const allSelected = conversations.length > 0 && selectedIds.length === conversations.length
  const mobile = variant === 'mobile'
  const drawer = variant === 'drawer'

  const finishRename = (conversation: AIConversation) => {
    const nextTitle = title.trim()
    if (nextTitle && nextTitle !== conversation.title)
      onRename(conversation.id, nextTitle)
    setRenamingId(undefined)
  }
  const toggleSelected = (id: string, checked: boolean) => {
    setSelectedKeys((current) => {
      const next = new Set(current)
      if (checked)
        next.add(id)
      else
        next.delete(id)
      return next
    })
  }
  const exitSelection = () => {
    setSelecting(false)
    setSelectedKeys(new Set())
  }
  const confirmDelete = async () => {
    const ids = deleteTargets.map(conversation => conversation.id)
    try {
      await onDeleteMany(ids)
      setSelectedKeys(current => new Set([...current].filter(id => !ids.includes(id))))
      if (ids.length > 1)
        setSelecting(false)
    }
    catch {
      // The parent mutation presents the localized error.
    }
    finally {
      setDeleteTargets([])
    }
  }

  return (
    <aside className={cn('flex size-full min-h-0 flex-col bg-surface', mobile ? 'relative z-20 flex-1 overflow-hidden rounded-t-feature border-t border-separator-subtle shadow-overlay' : 'shrink-0 border-r border-separator-subtle')}>
      {mobile
        ? (
            <div className="flex h-[calc(3.5rem+env(safe-area-inset-top))] shrink-0 items-center gap-1 border-b border-separator-subtle pb-0 pl-[max(0.5rem,env(safe-area-inset-left))] pr-[max(0.5rem,env(safe-area-inset-right))] pt-[env(safe-area-inset-top)]">
              <Button aria-label={t('common.back')} size="icon" variant="ghost" onClick={onBack}><ArrowLeft className="size-4" /></Button>
              <h2 className="min-w-0 flex-1 truncate text-sm font-semibold">{selecting ? t('aiAssistant.conversations.selectMode') : t('aiAssistant.conversations.title')}</h2>
              <Button className="h-9 px-2.5 text-xs" size="sm" variant="ghost" onClick={() => selecting ? exitSelection() : setSelecting(true)}>
                {selecting ? t('aiAssistant.conversations.exitSelect') : t('aiAssistant.conversations.manage')}
              </Button>
            </div>
          )
        : (
            <div className="flex h-14 items-center gap-1 border-b border-separator-subtle px-3">
              <h2 className="min-w-0 flex-1 truncate text-sm font-semibold">{selecting ? t('aiAssistant.conversations.selectMode') : t('aiAssistant.conversations.title')}</h2>
              {drawer && <Button autoFocus aria-label={t('common.back')} size="icon" variant="ghost" onClick={onBack}><ArrowLeft className="size-4" /></Button>}
            </div>
          )}
      <div className="grid gap-2 p-2">
        <div className="flex items-center gap-1.5">
          {!mobile && (
            <Button
              aria-label={selecting ? t('aiAssistant.conversations.exitSelect') : t('aiAssistant.conversations.select')}
              className="size-8 shrink-0"
              size="icon"
              variant="ghost"
              onClick={() => selecting ? exitSelection() : setSelecting(true)}
            >
              {selecting ? <X className="size-4" /> : <ListChecks className="size-4" />}
            </Button>
          )}
          <div className="relative min-w-0 flex-1">
            <Search className="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input aria-label={t('aiAssistant.conversations.search')} className={cn('pl-8 text-base', mobile ? 'h-10' : 'h-8 text-xs')} placeholder={t('aiAssistant.conversations.search')} value={search} onChange={event => onSearch(event.target.value)} />
          </div>
        </div>
        {selecting && (
          <div className="flex min-h-8 items-center gap-1 rounded-control bg-surface-subtle px-1.5">
            <Button
              className="h-7 px-2 text-xs"
              disabled={conversations.length === 0}
              size="sm"
              variant="ghost"
              onClick={() => setSelectedKeys(allSelected ? new Set() : new Set(visibleIds))}
            >
              <CheckSquare2 className="size-3.5" />
              {allSelected ? t('aiAssistant.conversations.clearSelection') : t('aiAssistant.conversations.selectAll')}
            </Button>
            <span className="min-w-0 flex-1 truncate text-right text-[10px] text-muted-foreground">
              {t('aiAssistant.conversations.selectedCount', { count: selectedIds.length })}
            </span>
            <Button
              aria-label={t('aiAssistant.conversations.deleteSelected')}
              className="size-7 text-danger hover:text-danger"
              disabled={selectedIds.length === 0 || deleting}
              size="icon"
              variant="ghost"
              onClick={() => setDeleteTargets(conversations.filter(conversation => selectedKeys.has(conversation.id)))}
            >
              {deleting ? <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" /> : <Trash2 className="size-3.5" />}
            </Button>
          </div>
        )}
      </div>
      <div className={cn('min-h-0 min-w-0 flex-1 overflow-x-hidden overflow-y-auto overscroll-x-none px-2', mobile ? 'pb-[max(0.5rem,env(safe-area-inset-bottom))]' : 'pb-2')}>
        {loading && <Skeleton className="h-12 w-full" />}
        {!loading && conversations.length === 0 && (
          <div className="grid min-h-48 place-items-center px-5 text-center">
            <div className="grid justify-items-center gap-2">
              <span className="grid size-10 place-items-center rounded-full bg-surface-subtle text-muted-foreground"><MessageSquareText className="size-4" /></span>
              <strong className="text-xs font-medium">{search ? t('aiAssistant.conversations.noResults') : t('aiAssistant.conversations.empty')}</strong>
              {!search && <p className="max-w-52 text-[11px] leading-5 text-muted-foreground">{t('aiAssistant.conversations.emptyDescription')}</p>}
            </div>
          </div>
        )}
        {conversations.map(conversation => (
          <div key={conversation.id} className={cn('group flex items-center gap-2 rounded-control border-l-2 border-transparent hover:bg-surface-subtle', mobile ? 'min-h-14 px-3 py-2' : 'px-2 py-1.5', activeId === conversation.id && 'border-l-primary bg-primary-subtle text-primary-text')}>
            {selecting && (
              <Checkbox
                aria-label={t('aiAssistant.conversations.selectConversation', { title: conversation.title })}
                checked={selectedKeys.has(conversation.id)}
                onCheckedChange={checked => toggleSelected(conversation.id, checked === true)}
              />
            )}
            {renamingId === conversation.id
              ? (
                  <Input
                    autoFocus
                    className="h-8 min-w-0 flex-1 text-base sm:text-xs"
                    value={title}
                    onBlur={() => finishRename(conversation)}
                    onChange={event => setTitle(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter')
                        event.currentTarget.blur()
                      if (event.key === 'Escape')
                        setRenamingId(undefined)
                    }}
                  />
                )
              : (
                  <button
                    aria-current={activeId === conversation.id ? 'true' : undefined}
                    className="min-w-0 flex-1 text-left"
                    type="button"
                    onClick={() => selecting ? toggleSelected(conversation.id, !selectedKeys.has(conversation.id)) : onSelect(conversation.id)}
                  >
                    <span className="flex min-w-0 items-center gap-1.5">
                      <strong className="min-w-0 flex-1 truncate text-xs font-medium">{conversation.title}</strong>
                      {conversation.titleSource === 'user' && <LockKeyhole aria-label={t('aiAssistant.conversations.manualTitleLocked')} className="size-3 shrink-0 text-muted-foreground" />}
                    </span>
                    <span className="flex min-w-0 items-center gap-1.5 text-[10px] text-muted-foreground">
                      <time dateTime={conversation.updatedAt}>{formatAIConversationTimestamp(conversation.updatedAt, i18n.language)}</time>
                      {runningConversationIds.has(conversation.id) && <LoaderCircle aria-label={t('aiAssistant.generating')} className="size-3 shrink-0 animate-spin text-primary motion-reduce:animate-pulse" />}
                    </span>
                  </button>
                )}
            {!selecting && (
              <DropdownMenu>
                <DropdownMenuTrigger asChild><Button aria-label={t('common.actions')} className={cn('shrink-0 self-center', mobile ? 'size-8 opacity-100' : 'size-7 opacity-0 group-hover:opacity-100 focus:opacity-100')} size="icon" variant="ghost"><Ellipsis className="size-4" /></Button></DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onClick={() => {
                    setTitle(conversation.title)
                    setRenamingId(conversation.id)
                  }}
                  >
                    <Pencil className="size-4" />
                    {t('common.edit')}
                  </DropdownMenuItem>
                  <DropdownMenuItem variant="destructive" onClick={() => setDeleteTargets([conversation])}>
                    <Trash2 className="size-4" />
                    {t('common.delete')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            )}
          </div>
        ))}
      </div>
      <ConfirmDialog
        confirmText={deleteTargets.length > 1 ? t('aiAssistant.conversations.deleteCount', { count: deleteTargets.length }) : t('common.delete')}
        description={deleteTargets.length > 1
          ? t('aiAssistant.conversations.deleteManyDescription', { count: deleteTargets.length })
          : t('aiAssistant.conversations.deleteDescription', { title: deleteTargets[0]?.title })}
        open={deleteTargets.length > 0}
        pending={deleting}
        title={deleteTargets.length > 1 ? t('aiAssistant.conversations.deleteManyTitle') : t('aiAssistant.conversations.deleteTitle')}
        onConfirm={confirmDelete}
        onOpenChange={open => !open && setDeleteTargets([])}
      />
    </aside>
  )
}
