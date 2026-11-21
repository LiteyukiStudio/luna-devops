import type { AppTemplateSummary, PlatformAppTemplate } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowDownWideNarrow, ArrowUpNarrowWide, Plus, SearchX, Settings2, Upload } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { api } from '@/api'
import { useSession } from '@/app/session-context'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { TemplateGridSkeleton } from '@/components/common/loading-states'
import { PageChromeTools } from '@/components/common/page-chrome'
import { PageShell } from '@/components/common/page-shell'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { isPlatformAdmin } from '@/lib/roles'
import { cn } from '@/lib/utils'
import { InstallTemplateSheet } from './install-template-sheet'
import { PlatformTemplateEditorDialog } from './platform-template-editor-dialog'
import { downloadTemplateBundleJson } from './platform-template-form'
import { PlatformTemplateImportDialog } from './platform-template-import-dialog'
import { PlatformTemplateManagerSheet } from './platform-template-manager-sheet'
import { TemplateCard } from './template-card'
import { templateCategoryLabel } from './template-text'

type SortBy = 'name' | 'popularity'
type SortOrder = 'asc' | 'desc'

const ALL_CATEGORIES = 'all'

export function AppTemplatesPage() {
  const { i18n, t } = useTranslation()
  const { user } = useSession()
  const queryClient = useQueryClient()
  const canManagePlatformTemplates = isPlatformAdmin(user?.role)
  const [searchParams, setSearchParams] = useSearchParams()
  const [search, setSearch] = useState('')
  const [category, setCategory] = useState(ALL_CATEGORIES)
  const [sortBy, setSortBy] = useState<SortBy>('popularity')
  const [sortOrder, setSortOrder] = useState<SortOrder>('desc')
  const [selectedTemplateOverride, setSelectedTemplateOverride] = useState<AppTemplateSummary | null>(null)
  const [managerOpen, setManagerOpen] = useState(false)
  const [editorOpen, setEditorOpen] = useState(false)
  const [editingTemplate, setEditingTemplate] = useState<PlatformAppTemplate | null>(null)
  const [importOpen, setImportOpen] = useState(false)
  const requestedTemplateId = searchParams.get('template')
  const templates = useQuery({ queryKey: ['app-templates'], queryFn: () => api.listAppTemplates() })
  const templateItems = templates.data ?? []
  const selectedTemplate = selectedTemplateOverride
    ?? templateItems.find(template => template.id === requestedTemplateId)
    ?? null

  const exportMutation = useMutation({
    mutationFn: (templateId: string) => api.exportPlatformAppTemplate(templateId),
    onSuccess: (bundle) => {
      downloadTemplateBundleJson(bundle)
      toast.success(t('appTemplatesPage.platformAdmin.exportedToast'))
    },
    onError: error => toast.error(error.message),
  })

  const keyword = search.trim().toLowerCase()
  const keywordMatches = keyword
    ? templateItems.filter((template) => {
        const componentTokens = template.components.flatMap(component => [component.name, component.key, component.image])
        return [
          template.name,
          template.slug,
          template.description,
          template.image,
          template.officialWebsite,
          template.officialRepository,
          ...componentTokens,
        ].some(value => value.toLowerCase().includes(keyword))
      })
    : templateItems
  const categoryCounts = new Map<string, number>()
  for (const template of keywordMatches)
    categoryCounts.set(template.category, (categoryCounts.get(template.category) ?? 0) + 1)
  const categories = Array.from(new Set(templateItems.map(template => template.category).filter(Boolean)))
    .sort((a, b) => templateCategoryLabel(a, t).localeCompare(templateCategoryLabel(b, t), i18n.language))
  const direction = sortOrder === 'asc' ? 1 : -1
  const visibleTemplates = keywordMatches
    .filter(template => category === ALL_CATEGORIES || template.category === category)
    .sort((a, b) => {
      if (sortBy === 'name') {
        const nameResult = a.name.localeCompare(b.name, i18n.language)
        return nameResult === 0 ? a.slug.localeCompare(b.slug) : direction * nameResult
      }
      const popularityResult = (a.popularityWeight ?? 0) - (b.popularityWeight ?? 0)
      return popularityResult === 0 ? a.name.localeCompare(b.name, i18n.language) : direction * popularityResult
    })
  const filtered = Boolean(keyword) || category !== ALL_CATEGORIES
  // 只有浏览全部且未搜索时才拆出“推荐”分区，筛选结果保持单一列表，避免同一模板出现两次或被分区打散。
  const featuredTemplates = filtered ? [] : visibleTemplates.filter(template => template.featured)
  const remainingTemplates = featuredTemplates.length > 0 ? visibleTemplates.filter(template => !template.featured) : visibleTemplates

  function closeInstallSheet() {
    setSelectedTemplateOverride(null)
    setSearchParams((current) => {
      const next = new URLSearchParams(current)
      next.delete('template')
      return next
    }, { replace: true })
  }

  function clearFilters() {
    setSearch('')
    setCategory(ALL_CATEGORIES)
  }

  function openCreateDialog() {
    setEditingTemplate(null)
    setEditorOpen(true)
  }

  function openEditDialog(template: PlatformAppTemplate) {
    setEditingTemplate(template)
    setEditorOpen(true)
  }

  async function openEditById(templateId: string) {
    try {
      const cached = queryClient.getQueryData<PlatformAppTemplate[]>(['platform-app-templates'])
      const foundCached = cached?.find(item => item.id === templateId || item.slug === templateId)
      if (foundCached) {
        closeInstallSheet()
        openEditDialog(foundCached)
        return
      }
      const list = await queryClient.fetchQuery({
        queryFn: ({ signal }) => api.listPlatformAppTemplates(undefined, signal),
        queryKey: ['platform-app-templates'],
      })
      const found = list.find(item => item.id === templateId || item.slug === templateId)
      if (found) {
        closeInstallSheet()
        openEditDialog(found)
      }
    }
    catch (error) {
      toast.error(error instanceof Error ? error.message : String(error))
    }
  }

  return (
    <PageShell spacing="compact" width="full">
      {canManagePlatformTemplates && (
        <PageChromeTools>
          <Button type="button" variant="outline" onClick={() => setManagerOpen(true)}>
            <Settings2 className="size-4" />
            {t('appTemplatesPage.platformAdmin.manage')}
          </Button>
          <Button type="button" variant="outline" onClick={() => setImportOpen(true)}>
            <Upload className="size-4" />
            {t('appTemplatesPage.platformAdmin.importJson')}
          </Button>
          <Button type="button" onClick={openCreateDialog}>
            <Plus className="size-4" />
            {t('appTemplatesPage.platformAdmin.create')}
          </Button>
        </PageChromeTools>
      )}
      <div className="flex flex-col gap-3 @2xl/workspace:flex-row @2xl/workspace:items-start @2xl/workspace:justify-between">
        <div aria-label={t('appTemplatesPage.categoryFilter')} className="flex min-w-0 flex-wrap gap-1.5" role="group">
          <CategoryChip
            count={keywordMatches.length}
            label={t('appTemplatesPage.allCategories')}
            selected={category === ALL_CATEGORIES}
            onSelect={() => setCategory(ALL_CATEGORIES)}
          />
          {categories.map(item => (
            <CategoryChip
              key={item}
              count={categoryCounts.get(item) ?? 0}
              label={templateCategoryLabel(item, t)}
              selected={category === item}
              onSelect={() => setCategory(item)}
            />
          ))}
        </div>
        <div className="flex min-w-0 shrink-0 flex-wrap items-center gap-2">
          <Input
            aria-label={t('appTemplatesPage.searchPlaceholder')}
            className="w-full @lg/workspace:w-64"
            placeholder={t('appTemplatesPage.searchPlaceholder')}
            type="search"
            value={search}
            onChange={event => setSearch(event.target.value)}
          />
          <Select
            value={sortBy}
            onValueChange={next => setSortBy(next === 'name' ? 'name' : 'popularity')}
          >
            <SelectTrigger
              aria-label={t('appTemplatesPage.sortBy')}
              className="min-w-32 flex-1 @lg/workspace:w-36 @lg/workspace:flex-none"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="popularity">{t('appTemplatesPage.sortByPopularity')}</SelectItem>
              <SelectItem value="name">{t('appTemplatesPage.sortByName')}</SelectItem>
            </SelectContent>
          </Select>
          <Button
            aria-label={t('appTemplatesPage.sortOrder')}
            size="icon"
            title={sortOrder === 'desc' ? t('appTemplatesPage.sortDesc') : t('appTemplatesPage.sortAsc')}
            type="button"
            variant="outline"
            onClick={() => setSortOrder(current => current === 'desc' ? 'asc' : 'desc')}
          >
            {sortOrder === 'desc' ? <ArrowDownWideNarrow aria-hidden="true" /> : <ArrowUpNarrowWide aria-hidden="true" />}
            <span className="sr-only">{sortOrder === 'desc' ? t('appTemplatesPage.sortDesc') : t('appTemplatesPage.sortAsc')}</span>
          </Button>
        </div>
      </div>

      {templates.isError && (
        <ErrorState
          retrying={templates.isFetching}
          title={templates.error.message}
          onRetry={() => void templates.refetch()}
        />
      )}
      {templates.isLoading && <TemplateGridSkeleton />}
      {templates.isSuccess && (
        visibleTemplates.length === 0
          ? (
              filtered
                ? (
                    <EmptyState
                      actions={<Button type="button" variant="outline" onClick={clearFilters}>{t('appTemplatesPage.clearFilters')}</Button>}
                      description={t('appTemplatesPage.noResultsDescription')}
                      icon={<SearchX className="size-5" />}
                      mode="filtered"
                      title={t('appTemplatesPage.noResultsTitle')}
                    />
                  )
                : (
                    <EmptyState
                      actions={canManagePlatformTemplates && (
                        <div className="flex flex-wrap items-center justify-center gap-2">
                          <Button type="button" variant="outline" onClick={() => setImportOpen(true)}>
                            <Upload className="size-4" />
                            {t('appTemplatesPage.platformAdmin.importJson')}
                          </Button>
                          <Button type="button" onClick={openCreateDialog}>
                            <Plus className="size-4" />
                            {t('appTemplatesPage.platformAdmin.create')}
                          </Button>
                        </div>
                      )}
                      description={t('appTemplatesPage.emptyDescription')}
                      title={t('appTemplatesPage.emptyTitle')}
                    />
                  )
            )
          : featuredTemplates.length > 0
            ? (
                <>
                  <TemplateSection templates={featuredTemplates} title={t('appTemplatesPage.featuredSection')} titleId="app-templates-featured" onOpen={setSelectedTemplateOverride} />
                  {remainingTemplates.length > 0 && (
                    <TemplateSection templates={remainingTemplates} title={t('appTemplatesPage.allTemplatesSection')} titleId="app-templates-all" onOpen={setSelectedTemplateOverride} />
                  )}
                </>
              )
            : <TemplateGrid templates={visibleTemplates} onOpen={setSelectedTemplateOverride} />
      )}

      <InstallTemplateSheet
        exporting={exportMutation.isPending && exportMutation.variables === selectedTemplate?.id}
        summary={selectedTemplate}
        onClose={closeInstallSheet}
        onEditPlatformTemplate={canManagePlatformTemplates ? templateId => void openEditById(templateId) : undefined}
        onExportTemplate={canManagePlatformTemplates ? templateId => exportMutation.mutate(templateId) : undefined}
      />

      {canManagePlatformTemplates && (
        <>
          <PlatformTemplateManagerSheet
            exportingId={exportMutation.isPending ? (exportMutation.variables ?? null) : null}
            open={managerOpen}
            onCreate={openCreateDialog}
            onEdit={openEditDialog}
            onExport={templateId => exportMutation.mutate(templateId)}
            onImport={() => setImportOpen(true)}
            onOpenChange={setManagerOpen}
          />
          <PlatformTemplateEditorDialog
            open={editorOpen}
            template={editingTemplate}
            onOpenChange={(nextOpen) => {
              setEditorOpen(nextOpen)
              if (!nextOpen)
                setEditingTemplate(null)
            }}
          />
          <PlatformTemplateImportDialog
            open={importOpen}
            onOpenChange={setImportOpen}
          />
        </>
      )}
    </PageShell>
  )
}

function TemplateSection({ templates, title, titleId, onOpen }: { templates: AppTemplateSummary[], title: string, titleId: string, onOpen: (template: AppTemplateSummary) => void }) {
  return (
    <section aria-labelledby={titleId} className="grid gap-2.5">
      <h2 className="text-sm font-semibold" id={titleId}>{title}</h2>
      <TemplateGrid templates={templates} onOpen={onOpen} />
    </section>
  )
}

function TemplateGrid({ templates, onOpen }: { templates: AppTemplateSummary[], onOpen: (template: AppTemplateSummary) => void }) {
  return (
    <div className="grid gap-3 @lg/workspace:grid-cols-2 @4xl/workspace:grid-cols-3 @6xl/workspace:grid-cols-4">
      {templates.map(template => (
        <TemplateCard
          key={template.id}
          template={template}
          onOpen={() => onOpen(template)}
        />
      ))}
    </div>
  )
}

function CategoryChip({ count, label, selected, onSelect }: { count: number, label: string, selected: boolean, onSelect: () => void }) {
  return (
    <button
      aria-pressed={selected}
      className={cn(
        'inline-flex h-7 items-center gap-1.5 rounded-full border px-3 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-primary/50',
        selected
          ? 'border-primary-border bg-primary-subtle text-primary-text'
          : 'border-border bg-surface-raised text-muted-foreground hover:bg-muted hover:text-foreground',
      )}
      type="button"
      onClick={onSelect}
    >
      {label}
      {' '}
      <span className={cn('tabular-nums', !selected && 'text-muted-foreground/80')}>{count}</span>
    </button>
  )
}
