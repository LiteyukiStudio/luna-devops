import type { PlatformAppTemplate } from '@/api'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Download, PackagePlus, Pencil, Plus, Power, Trash2, Upload } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { api } from '@/api'
import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { EmptyState } from '@/components/common/empty-state'
import { ErrorState } from '@/components/common/error-state'
import { StatusBadge } from '@/components/common/status-badge'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { platformTemplateToWritePayload } from './platform-template-form'
import { TemplateIcon } from './template-card'
import { templateCategoryLabel } from './template-text'

export function PlatformTemplateManagerSheet({
  exportingId,
  open,
  onCreate,
  onEdit,
  onExport,
  onImport,
  onOpenChange,
}: {
  exportingId: string | null
  open: boolean
  onCreate: () => void
  onEdit: (template: PlatformAppTemplate) => void
  onExport: (templateId: string) => void
  onImport: () => void
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const templatesQuery = useQuery({
    enabled: open,
    queryFn: ({ signal }) => api.listPlatformAppTemplates(undefined, signal),
    queryKey: ['platform-app-templates'],
  })

  const toggleMutation = useMutation({
    mutationFn: (template: PlatformAppTemplate) =>
      api.updatePlatformAppTemplate(
        template.id,
        platformTemplateToWritePayload(template, { enabled: !template.enabled }),
      ),
    onSuccess: async (updated) => {
      toast.success(updated.enabled ? t('appTemplatesPage.platformAdmin.enabledToast') : t('appTemplatesPage.platformAdmin.disabledToast'))
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['app-templates'] }),
        queryClient.invalidateQueries({ queryKey: ['app-template'] }),
        queryClient.invalidateQueries({ queryKey: ['platform-app-templates'] }),
      ])
    },
    onError: error => toast.error(error.message),
  })

  const deleteMutation = useMutation({
    mutationFn: (templateId: string) => api.deletePlatformAppTemplate(templateId),
    onSuccess: async () => {
      toast.success(t('appTemplatesPage.platformAdmin.deletedToast'))
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['app-templates'] }),
        queryClient.invalidateQueries({ queryKey: ['app-template'] }),
        queryClient.invalidateQueries({ queryKey: ['platform-app-templates'] }),
      ])
    },
    onError: error => toast.error(error.message),
  })

  const items = templatesQuery.data ?? []

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 p-0 sm:max-w-xl">
        <SheetHeader className="shrink-0 gap-3 border-b border-border px-5 py-4 pr-14">
          <SheetTitle className="text-base">{t('appTemplatesPage.platformAdmin.managerTitle')}</SheetTitle>
          <SheetDescription className="leading-5">
            {t('appTemplatesPage.platformAdmin.managerDescription')}
          </SheetDescription>
          <div className="flex flex-wrap items-center gap-2 pt-1">
            <Button size="sm" type="button" variant="outline" onClick={onImport}>
              <Upload className="size-3.5" />
              {t('appTemplatesPage.platformAdmin.importJson')}
            </Button>
            <Button size="sm" type="button" onClick={onCreate}>
              <Plus className="size-3.5" />
              {t('appTemplatesPage.platformAdmin.create')}
            </Button>
          </div>
        </SheetHeader>

        <div className="grid min-h-0 flex-1 content-start gap-3 overflow-y-auto p-5">
          {templatesQuery.isError && <ErrorState title={templatesQuery.error.message} />}
          {templatesQuery.isLoading && (
            <div aria-busy="true" className="grid gap-3">
              {Array.from({ length: 3 }, (_, index) => (
                <Skeleton key={index} className="h-24 w-full rounded-container" />
              ))}
            </div>
          )}
          {templatesQuery.isSuccess && items.length === 0 && (
            <EmptyState
              actions={(
                <div className="flex flex-wrap items-center justify-center gap-2">
                  <Button size="sm" type="button" variant="outline" onClick={onImport}>
                    <Upload className="size-3.5" />
                    {t('appTemplatesPage.platformAdmin.importJson')}
                  </Button>
                  <Button size="sm" type="button" onClick={onCreate}>
                    <Plus className="size-3.5" />
                    {t('appTemplatesPage.platformAdmin.create')}
                  </Button>
                </div>
              )}
              description={t('appTemplatesPage.platformAdmin.managerEmptyDescription')}
              icon={<PackagePlus className="size-5" />}
              title={t('appTemplatesPage.platformAdmin.managerEmptyTitle')}
            />
          )}
          {templatesQuery.isSuccess && items.map((item) => {
            const togglingThis = toggleMutation.isPending && toggleMutation.variables?.id === item.id
            const deletingThis = deleteMutation.isPending && deleteMutation.variables === item.id
            const exportingThis = exportingId === item.id
            return (
              <div
                key={item.id}
                className="grid gap-3 rounded-container border border-border bg-surface-raised p-4 shadow-sm"
              >
                <div className="flex min-w-0 items-start gap-3">
                  <TemplateIcon icon={item.icon} />
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="truncate text-sm font-semibold text-foreground">{item.name}</span>
                      <span className="font-mono text-xs text-muted-foreground">{item.slug}</span>
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                      <span>{templateCategoryLabel(item.category, t)}</span>
                      {item.version && <span className="font-mono">{item.version}</span>}
                      {item.isMultiComponent
                        ? <span>{t('appTemplatesPage.componentCount', { count: item.components.length })}</span>
                        : <span className="truncate font-mono">{item.image}</span>}
                    </div>
                  </div>
                  <StatusBadge className="shrink-0" tone={item.enabled ? 'success' : 'neutral'}>
                    {item.enabled ? t('appTemplatesPage.platformAdmin.enabledBadge') : t('appTemplatesPage.platformAdmin.disabledBadge')}
                  </StatusBadge>
                </div>
                {item.description && (
                  <p className="line-clamp-2 text-xs leading-5 text-muted-foreground">{item.description}</p>
                )}
                <div className="flex flex-wrap items-center justify-end gap-2 border-t border-border pt-3">
                  <Button
                    disabled={togglingThis || deletingThis}
                    size="sm"
                    type="button"
                    variant="outline"
                    onClick={() => toggleMutation.mutate(item)}
                  >
                    <Power className="size-3.5" />
                    {item.enabled ? t('appTemplatesPage.platformAdmin.disableAction') : t('appTemplatesPage.platformAdmin.enableAction')}
                  </Button>
                  <Button
                    disabled={deletingThis}
                    size="sm"
                    type="button"
                    variant="outline"
                    onClick={() => onEdit(item)}
                  >
                    <Pencil className="size-3.5" />
                    {t('appTemplatesPage.platformAdmin.edit')}
                  </Button>
                  <Button
                    disabled={exportingThis || deletingThis}
                    size="sm"
                    type="button"
                    variant="outline"
                    onClick={() => onExport(item.id)}
                  >
                    <Download className="size-3.5" />
                    {t('appTemplatesPage.platformAdmin.exportJson')}
                  </Button>
                  <ConfirmDialog
                    confirmText={t('common.delete')}
                    description={t('appTemplatesPage.platformAdmin.deleteDescription')}
                    pending={deletingThis}
                    title={t('appTemplatesPage.platformAdmin.deleteTitle', { name: item.name })}
                    onConfirm={async () => {
                      await deleteMutation.mutateAsync(item.id)
                    }}
                  >
                    <Button disabled={deletingThis} size="sm" type="button" variant="outline">
                      <Trash2 className="size-3.5 text-danger" />
                      {t('common.delete')}
                    </Button>
                  </ConfirmDialog>
                </div>
              </div>
            )
          })}
        </div>
      </SheetContent>
    </Sheet>
  )
}
