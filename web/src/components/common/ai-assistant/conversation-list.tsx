import type { AIConversation } from '@/api'
import { ArrowLeft, CheckSquare2, Ellipsis, ListChecks, LoaderCircle, LockKeyhole, MessageSquareText, Pencil, Search, Trash2, X } from 'lucide-react'
import { useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { formatAIConversationTimestamp, groupAIConversationsByRecency } from './conversation-timestamp'
import { useInfiniteLoadTrigger } from './infinite-load-trigger'

const conversationSkeletonKeys = [
  'conversation-skeleton-1',
  'conversation-skeleton-2',
  'conversation-skeleton-3',
  'conversation-skeleton-4',
  'conversation-skeleton-5',
]

export interface AIConversationListProps {
  activeId?: string
  conversations: AIConversation[]
  deleting: boolean
  error?: unknown
  hasMore?: boolean
  loading: boolean
  loadingMore?: boolean
  runningConversationIds: Set<string>
  search: string
  surface?: 'page' | 'window'
  variant?: 'drawer' | 'sidebar' | 'mobile'
  onBack?: () => void
  onClearSearch?: () => void
  onDeleteMany: (ids: string[]) => Promise<void>
  onLoadMore?: () => Promise<void>
  onRename: (id: string, title: string) => void
  onRetry?: () => void
  onSearch: (search: string) => void
  onSelect: (id: string) => void
}

export function AIConversationList({
  activeId,
  conversations,
  deleting,
  error,
  hasMore = false,
  loading,
  loadingMore = false,
  runningConversationIds,
  search,
  surface = 'window',
  variant = 'sidebar',
  onBack,
  onClearSearch,
  onDeleteMany,
  onLoadMore = async () => {},
  onRename,
  onRetry,
  onSearch,
  onSelect,
}: AIConversationListProps) {
  const { t, i18n } = useTranslation()
  const [selecting, setSelecting] = useState(false)
  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(() => new Set())
  const [renamingId, setRenamingId] = useState<string>()
  const [deleteTargets, setDeleteTargets] = useState<AIConversation[]>([])
  const [title, setTitle] = useState('')
  const viewportRef = useRef<HTMLDivElement>(null)
  const moreTrigger = useInfiniteLoadTrigger({
    enabled: hasMore,
    loading: loadingMore,
    onLoad: onLoadMore,
    rootRef: viewportRef,
  })
  const visibleIds = useMemo(() => conversations.map(conversation => conversation.id), [conversations])
  const selectedIds = visibleIds.filter(id => selectedKeys.has(id))
  const allSelected = conversations.length > 0 && selectedIds.length === conversations.length
  const page = surface === 'page'
  const mobile = variant === 'mobile' || page
  const drawer = variant === 'drawer' && !page
  const Title = page ? 'h1' : 'h2'
  const failed = Boolean(error)
  const clearSearch = onClearSearch ?? (() => onSearch(''))
  const groups = groupAIConversationsByRecency(conversations)
  const groupLabel = (recency: 'today' | 'recent' | 'earlier') => {
    if (recency === 'today')
      return t('aiAssistant.conversations.groups.today')
    if (recency === 'recent')
      return t('aiAssistant.conversations.groups.recent')
    return t('aiAssistant.conversations.groups.earlier')
  }

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
    <aside
      className={cn(
        'flex size-full min-h-0 flex-col bg-surface',
        page
          ? 'relative flex-1 overflow-hidden'
          : mobile
            ? 'relative z-20 flex-1 overflow-hidden'
            : drawer ? 'shrink-0' : 'shrink-0 border-r border-border bg-surface-subtle',
      )}
      data-ai-conversation-surface={surface}
    >
      {mobile
        ? (
            <div className={cn(
              'flex shrink-0 items-center gap-1 border-b border-border pb-0 pt-[env(safe-area-inset-top)]',
              page
                ? 'h-[calc(3.5rem+env(safe-area-inset-top))] pl-[max(0.5rem,env(safe-area-inset-left))] pr-[max(0.5rem,env(safe-area-inset-right))] pointer-fine:h-[calc(3rem+env(safe-area-inset-top))]'
                : 'h-[calc(3.5rem+env(safe-area-inset-top))] pl-[max(0.5rem,env(safe-area-inset-left))] pr-[max(0.5rem,env(safe-area-inset-right))]',
            )}
            >
              <Button aria-label={t('common.back')} className={page ? 'size-11 pointer-fine:size-9' : 'size-9'} size="icon" variant="ghost" onClick={onBack}><ArrowLeft className="size-4" /></Button>
              <Title className="min-w-0 flex-1 truncate text-sm font-medium">{selecting ? t('aiAssistant.conversations.selectMode') : t('aiAssistant.conversations.title')}</Title>
              <Button className={page ? 'min-h-11 px-3 text-sm pointer-fine:h-8 pointer-fine:min-h-0' : 'h-8 px-2.5 text-xs'} size="sm" variant="ghost" onClick={() => selecting ? exitSelection() : setSelecting(true)}>
                {selecting ? t('aiAssistant.conversations.exitSelect') : t('aiAssistant.conversations.manage')}
              </Button>
            </div>
          )
        : (
            <div className="flex h-12 shrink-0 items-center gap-1 border-b border-border pl-3.5 pr-2">
              <Title className="min-w-0 flex-1 truncate text-sm font-medium">{selecting ? t('aiAssistant.conversations.selectMode') : t('aiAssistant.conversations.title')}</Title>
              {drawer && <Button autoFocus aria-label={t('common.back')} className="size-8" size="icon" variant="ghost" onClick={onBack}><ArrowLeft className="size-4" /></Button>}
            </div>
          )}
      <div
        className={cn(
          'grid gap-2 py-2',
          page
            ? 'pl-[max(0.5rem,env(safe-area-inset-left))] pr-[max(0.5rem,env(safe-area-inset-right))]'
            : 'px-2',
        )}
        data-slot="ai-conversation-toolbar"
      >
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
            <Search aria-hidden="true" className="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input aria-label={t('aiAssistant.conversations.search')} className={cn('bg-surface pl-8 text-base', page ? 'h-11 pr-12 pointer-fine:h-8 pointer-fine:pr-9 pointer-fine:text-sm' : mobile ? 'h-10' : 'h-8 pr-8 sm:text-xs')} placeholder={t('aiAssistant.conversations.search')} value={search} onChange={event => onSearch(event.target.value)} />
            {search && (page || onClearSearch) && (
              <Button
                aria-label={t('aiAssistant.conversations.clearSearch')}
                className={page ? 'absolute right-0 top-1/2 size-11 -translate-y-1/2 pointer-fine:size-8' : 'absolute right-0 top-1/2 size-8 -translate-y-1/2'}
                size="icon"
                variant="ghost"
                onClick={clearSearch}
              >
                <X className="size-4" />
              </Button>
            )}
          </div>
        </div>
        {selecting && (
          <div className={page ? 'flex min-h-11 items-center gap-2 rounded-control border border-border bg-surface px-2' : 'flex min-h-8 items-center gap-1 rounded-control border border-border bg-surface px-1.5'}>
            <Button
              className={page ? 'min-h-11 px-3 text-sm' : 'h-7 px-2 text-xs'}
              disabled={conversations.length === 0}
              size="sm"
              variant="ghost"
              onClick={() => setSelectedKeys(allSelected ? new Set() : new Set(visibleIds))}
            >
              <CheckSquare2 className="size-3.5" />
              {allSelected ? t('aiAssistant.conversations.clearSelection') : t('aiAssistant.conversations.selectAll')}
            </Button>
            <span className={page ? 'min-w-0 flex-1 truncate text-right text-xs text-muted-foreground' : 'min-w-0 flex-1 truncate text-right text-[11px] text-muted-foreground'}>
              {t('aiAssistant.conversations.selectedCount', { count: selectedIds.length })}
            </span>
            <Button
              aria-label={t('aiAssistant.conversations.deleteSelected')}
              className={page ? 'size-12 text-danger hover:text-danger' : 'size-7 text-danger hover:text-danger'}
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
      <div
        ref={viewportRef}
        className={cn(
          'min-h-0 min-w-0 flex-1 overflow-x-hidden overflow-y-auto overscroll-x-none',
          page
            ? 'pb-[max(0.5rem,env(safe-area-inset-bottom))] pl-[max(0.5rem,env(safe-area-inset-left))] pr-[max(0.5rem,env(safe-area-inset-right))]'
            : 'px-2',
          !page && (mobile ? 'pb-[max(0.5rem,env(safe-area-inset-bottom))]' : 'pb-2'),
        )}
        data-slot="ai-conversation-list"
      >
        {loading && (page
          ? <div aria-label={t('common.loading')} className="grid gap-2" role="status">{conversationSkeletonKeys.map(key => <Skeleton key={key} className="h-16 w-full" />)}</div>
          : <Skeleton className="h-12 w-full" />)}
        {!loading && failed && conversations.length === 0 && (
          <div className="grid min-h-48 place-items-center px-5 text-center" role="alert">
            <div className="grid justify-items-center gap-3">
              <strong className={page ? 'text-sm font-medium' : 'text-xs font-medium'}>{t('aiAssistant.conversations.loadError')}</strong>
              {onRetry && <Button className={page ? 'min-h-11 px-4' : undefined} size="sm" variant="outline" onClick={onRetry}>{t('common.retry')}</Button>}
            </div>
          </div>
        )}
        {!loading && !failed && conversations.length === 0 && (
          <div className="grid min-h-48 place-items-center px-5 text-center">
            <div className="grid justify-items-center gap-2">
              <span className="grid size-10 place-items-center rounded-full border border-border bg-surface text-muted-foreground"><MessageSquareText className="size-4" /></span>
              <strong className={page ? 'text-sm font-medium' : 'text-xs font-medium'}>{search ? t('aiAssistant.conversations.noResults') : t('aiAssistant.conversations.empty')}</strong>
              {!search && <p className={page ? 'max-w-64 text-sm leading-6 text-muted-foreground' : 'max-w-52 text-xs leading-5 text-muted-foreground'}>{t('aiAssistant.conversations.emptyDescription')}</p>}
              {search && page && <Button className="min-h-11 px-4" size="sm" variant="outline" onClick={clearSearch}>{t('aiAssistant.conversations.clearSearch')}</Button>}
            </div>
          </div>
        )}
        {!loading && failed && conversations.length > 0 && (
          <div className="mb-2 flex min-h-11 items-center gap-2 rounded-control border border-danger-border bg-danger-subtle px-3 text-sm text-danger" role="alert">
            <span className="min-w-0 flex-1">{t('aiAssistant.conversations.loadError')}</span>
            {onRetry && <Button className={page ? 'min-h-11' : undefined} size="sm" variant="outline" onClick={onRetry}>{t('common.retry')}</Button>}
          </div>
        )}
        {groups.map(group => (
          <section key={group.recency} aria-label={groupLabel(group.recency)} className="grid gap-px" data-ai-conversation-group={group.recency}>
            <p aria-hidden="true" className={cn('px-2 pb-1 pt-3 text-[11px] font-medium text-muted-foreground', page && 'text-xs')}>{groupLabel(group.recency)}</p>
            {group.items.map(conversation => (
              <div key={conversation.id} className={cn('group flex items-center gap-2 rounded-control transition-colors duration-fast hover:bg-muted', page ? 'min-h-14 px-2 py-1.5 pointer-fine:min-h-11' : mobile ? 'min-h-14 px-3 py-2' : 'min-h-11 px-2 py-1', activeId === conversation.id && 'bg-primary-subtle text-primary-text hover:bg-primary-subtle')}>
                {selecting && (
                  page
                    ? (
                        <label className="grid size-11 shrink-0 cursor-pointer place-items-center">
                          <Checkbox
                            aria-label={t('aiAssistant.conversations.selectConversation', { title: conversation.title })}
                            checked={selectedKeys.has(conversation.id)}
                            onCheckedChange={checked => toggleSelected(conversation.id, checked === true)}
                          />
                        </label>
                      )
                    : (
                        <Checkbox
                          aria-label={t('aiAssistant.conversations.selectConversation', { title: conversation.title })}
                          checked={selectedKeys.has(conversation.id)}
                          onCheckedChange={checked => toggleSelected(conversation.id, checked === true)}
                        />
                      )
                )}
                {renamingId === conversation.id
                  ? (
                      <Input
                        autoFocus
                        className={page ? 'h-11 min-w-0 flex-1 text-base' : 'h-8 min-w-0 flex-1 text-base sm:text-xs'}
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
                        className={page ? 'min-h-11 min-w-0 flex-1 py-1 text-left' : 'min-w-0 flex-1 text-left'}
                        type="button"
                        onClick={() => selecting ? toggleSelected(conversation.id, !selectedKeys.has(conversation.id)) : onSelect(conversation.id)}
                      >
                        <span className="flex min-w-0 items-center gap-1.5">
                          <strong className={page ? 'min-w-0 flex-1 truncate text-sm font-medium' : 'min-w-0 flex-1 truncate text-[13px] font-medium'}>{conversation.title}</strong>
                          {conversation.titleSource === 'user' && <LockKeyhole aria-label={t('aiAssistant.conversations.manualTitleLocked')} className="size-3 shrink-0 text-muted-foreground" />}
                        </span>
                        <span className={page ? 'flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground' : 'flex min-w-0 items-center gap-1.5 text-[11px] text-muted-foreground'}>
                          <time dateTime={conversation.updatedAt}>{formatAIConversationTimestamp(conversation.updatedAt, i18n.language)}</time>
                          {runningConversationIds.has(conversation.id) && <LoaderCircle aria-label={t('aiAssistant.generating')} className="size-3 shrink-0 animate-spin text-primary motion-reduce:animate-pulse" />}
                        </span>
                      </button>
                    )}
                {!selecting && (
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild><Button aria-label={t('common.actions')} className={cn('shrink-0 self-center text-muted-foreground', page ? 'size-11 opacity-100 pointer-fine:size-8' : mobile ? 'size-8 opacity-100' : 'size-7 opacity-0 group-hover:opacity-100 focus:opacity-100 data-[state=open]:opacity-100 [@media(hover:none)]:opacity-100')} size="icon" variant="ghost"><Ellipsis className="size-4" /></Button></DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className={page ? '[&_[data-slot=dropdown-menu-item]]:min-h-11' : undefined}>
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
          </section>
        ))}
        {hasMore && (
          <div ref={moreTrigger.sentinelRef} className="flex min-h-10 items-center justify-center py-1" data-ai-conversation-sentinel>
            <Button
              className={page ? 'min-h-11 gap-2 px-3 text-sm text-muted-foreground pointer-fine:min-h-8' : 'h-7 gap-1.5 px-2.5 text-xs text-muted-foreground'}
              disabled={loadingMore}
              size="sm"
              variant="ghost"
              onClick={() => void moreTrigger.load()}
            >
              {loadingMore && <LoaderCircle className="size-3 animate-spin motion-reduce:animate-none" />}
              {t(loadingMore ? 'aiAssistant.conversations.loadingMore' : 'aiAssistant.conversations.loadMore')}
            </Button>
          </div>
        )}
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
