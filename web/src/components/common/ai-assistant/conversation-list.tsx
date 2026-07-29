import type { AIConversation } from '@/api'
import { CheckSquare2, ChevronRight, Ellipsis, ListChecks, LoaderCircle, LockKeyhole, MessageSquarePlus, Pencil, Search, Trash2, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

export interface AIConversationListProps {
  activeId?: string
  conversations: AIConversation[]
  deleting: boolean
  loading: boolean
  runningConversationIds: Set<string>
  search: string
  onClose: () => void
  onCreate: () => void
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
  onClose,
  onCreate,
  onDeleteMany,
  onRename,
  onSearch,
  onSelect,
}: AIConversationListProps) {
  const { t } = useTranslation()
  const [selecting, setSelecting] = useState(false)
  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(() => new Set())
  const [renamingId, setRenamingId] = useState<string>()
  const [deleteTargets, setDeleteTargets] = useState<AIConversation[]>([])
  const [title, setTitle] = useState('')
  const visibleIds = useMemo(() => conversations.map(conversation => conversation.id), [conversations])
  const selectedIds = visibleIds.filter(id => selectedKeys.has(id))
  const allSelected = conversations.length > 0 && selectedIds.length === conversations.length

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
    <aside className="absolute inset-0 z-10 flex flex-col bg-surface sm:static sm:w-64 sm:shrink-0 sm:border-r sm:border-separator-subtle">
      <div className="flex h-14 items-center gap-2 border-b border-separator-subtle px-3">
        <Button aria-label={t('common.close')} size="icon" variant="ghost" onClick={onClose}><ChevronRight className="size-4 rotate-180" /></Button>
        <h2 className="min-w-0 flex-1 truncate text-sm font-semibold">{selecting ? t('aiAssistant.conversations.selectMode') : t('aiAssistant.conversations.title')}</h2>
        <Button
          aria-label={selecting ? t('aiAssistant.conversations.exitSelect') : t('aiAssistant.conversations.select')}
          size="icon"
          variant="ghost"
          onClick={() => selecting ? exitSelection() : setSelecting(true)}
        >
          {selecting ? <X className="size-4" /> : <ListChecks className="size-4" />}
        </Button>
        {!selecting && <Button aria-label={t('aiAssistant.conversations.new')} size="icon" variant="ghost" onClick={onCreate}><MessageSquarePlus className="size-4" /></Button>}
      </div>
      <div className="grid gap-2 p-2">
        <div className="relative">
          <Search className="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input aria-label={t('aiAssistant.conversations.search')} className="h-8 pl-8 text-xs" placeholder={t('aiAssistant.conversations.search')} value={search} onChange={event => onSearch(event.target.value)} />
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
      <div className="min-h-0 min-w-0 flex-1 overflow-x-hidden overflow-y-auto overscroll-x-none px-2 pb-2">
        {loading && <Skeleton className="h-12 w-full" />}
        {conversations.map(conversation => (
          <div key={conversation.id} className={cn('group flex items-center gap-2 rounded-control px-2 py-1.5 hover:bg-surface-subtle', activeId === conversation.id && 'bg-primary-subtle text-primary-text')}>
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
                    className="h-8 min-w-0 flex-1 text-xs"
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
                    className="min-w-0 flex-1 text-left"
                    type="button"
                    onClick={() => selecting ? toggleSelected(conversation.id, !selectedKeys.has(conversation.id)) : onSelect(conversation.id)}
                  >
                    <span className="flex min-w-0 items-center gap-1.5">
                      <strong className="min-w-0 flex-1 truncate text-xs font-medium">{conversation.title}</strong>
                      {conversation.titleSource === 'user' && <LockKeyhole aria-label={t('aiAssistant.conversations.manualTitleLocked')} className="size-3 shrink-0 text-muted-foreground" />}
                      {runningConversationIds.has(conversation.id) && <LoaderCircle aria-label={t('aiAssistant.generating')} className="size-3.5 shrink-0 animate-spin text-primary-text motion-reduce:animate-pulse" />}
                    </span>
                    <span className="block text-[10px] text-muted-foreground">{new Date(conversation.updatedAt).toLocaleString()}</span>
                  </button>
                )}
            {!selecting && (
              <DropdownMenu>
                <DropdownMenuTrigger asChild><Button aria-label={t('common.actions')} className="size-7 shrink-0 opacity-0 group-hover:opacity-100 focus:opacity-100" size="icon" variant="ghost"><Ellipsis className="size-3.5" /></Button></DropdownMenuTrigger>
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
