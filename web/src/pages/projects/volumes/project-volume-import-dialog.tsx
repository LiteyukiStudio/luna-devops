import type { ProjectVolumeAccessMode, ProjectVolumeMode, VolumeTransferFormat } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { Upload } from 'lucide-react'
import { useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { FormField as Field } from '@/components/common/form-field'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { ProjectVolumeClusterSelect, ProjectVolumeStorageClassSelect } from './volume-resource-selectors'
import { sha256File, uploadVolumeImport, waitForVolumeTransferReady } from './volume-transfer-upload'

interface ImportValues {
  accessMode: ProjectVolumeAccessMode
  capacity: string
  clusterId: string
  displayName: string
  format: VolumeTransferFormat
  storageClassName: string
  volumeMode: ProjectVolumeMode
}

type ImportStage = 'cancelling' | 'hashing' | 'idle' | 'preparing' | 'uploading' | 'verifying'

const defaults: ImportValues = {
  accessMode: 'ReadWriteOnce',
  capacity: '10Gi',
  clusterId: '',
  displayName: '',
  format: 'tar_gz',
  storageClassName: '',
  volumeMode: 'Filesystem',
}

function formatBytes(value: number) {
  if (value < 1024)
    return `${value} B`
  const units = ['KiB', 'MiB', 'GiB', 'TiB']
  let amount = value
  let index = -1
  do {
    amount /= 1024
    index += 1
  } while (amount >= 1024 && index < units.length - 1)
  return `${amount.toFixed(amount >= 10 ? 1 : 2)} ${units[index]}`
}

function importSchema(required: string, nameTooLong: string) {
  return z.object({
    accessMode: z.enum(['ReadWriteOnce', 'ReadWriteOncePod', 'ReadOnlyMany', 'ReadWriteMany']),
    capacity: z.string().trim().min(1, required),
    clusterId: z.string().min(1, required),
    displayName: z.string().trim().min(1, required).max(120, nameTooLong),
    format: z.enum(['tar_gz', 'raw_zst']),
    storageClassName: z.string().min(1, required),
    volumeMode: z.enum(['Filesystem', 'Block']),
  })
}

export function ProjectVolumeImportDialog({ onOpenChange, open, projectId }: { onOpenChange: (open: boolean) => void, open: boolean, projectId: string }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [file, setFile] = useState<File | null>(null)
  const [progress, setProgress] = useState(0)
  const [stage, setStage] = useState<ImportStage>('idle')
  const [activeTransferId, setActiveTransferId] = useState<string | null>(null)
  const abortRef = useRef<AbortController | null>(null)
  const cancelIssuedForRef = useRef<string | null>(null)
  const form = useForm<ImportValues>({ defaultValues: defaults, mode: 'onChange', resolver: zodResolver(importSchema(t('common.required'), t('projectVolumes.nameTooLong'))) })
  const clusterId = form.watch('clusterId')
  const volumeMode = form.watch('volumeMode')
  const importFormat: VolumeTransferFormat = volumeMode === 'Block' ? 'raw_zst' : 'tar_gz'

  async function startImport(values: ImportValues) {
    if (!file) {
      toast.error(t('projectVolumes.fileRequired'))
      return
    }
    if (file.size === 0) {
      toast.error(t('projectVolumes.fileEmpty'))
      return
    }
    const controller = new AbortController()
    abortRef.current = controller
    cancelIssuedForRef.current = null
    let transferId: string | null = null
    try {
      setStage('hashing')
      setProgress(0)
      const sha256 = await sha256File(file, (processed, total) => setProgress(total > 0 ? (processed / total) * 100 : 100), controller.signal)
      const result = await api.createVolumeImport(projectId, {
        ...values,
        displayName: values.displayName.trim(),
        capacity: values.capacity.trim(),
        filename: file.name,
        contentLength: file.size,
        sha256,
      })
      transferId = result.transfer.id
      setActiveTransferId(transferId)
      setStage('preparing')
      setProgress(0)
      await waitForVolumeTransferReady({ projectId, transferId, signal: controller.signal })
      setStage('uploading')
      await uploadVolumeImport({
        file,
        projectId,
        sha256,
        signal: controller.signal,
        transferId,
        onProgress: (current) => {
          setProgress(current.percent)
          if (current.transferredBytes >= current.total)
            setStage('verifying')
        },
      })
      toast.success(t('projectVolumes.importSucceeded'))
      queryClient.invalidateQueries({ queryKey: ['project-volumes', projectId] })
      queryClient.invalidateQueries({ queryKey: ['project-volume-transfers', projectId] })
      onOpenChange(false)
      form.reset(defaults)
      setFile(null)
      setProgress(0)
      setStage('idle')
    }
    catch (error) {
      if (controller.signal.aborted && transferId && cancelIssuedForRef.current !== transferId)
        await api.cancelVolumeTransfer(projectId, transferId).catch(() => undefined)
      if (!controller.signal.aborted)
        toast.error(t('projectVolumes.importInterrupted'), { description: error instanceof Error ? error.message : t('errors.request.failed') })
      setStage('idle')
    }
    finally {
      setActiveTransferId(null)
      if (abortRef.current === controller)
        abortRef.current = null
    }
  }

  async function cancelCurrentImport() {
    const transferId = activeTransferId
    setStage('cancelling')
    abortRef.current?.abort()
    try {
      if (transferId) {
        cancelIssuedForRef.current = transferId
        await api.cancelVolumeTransfer(projectId, transferId)
      }
      toast.success(t('projectVolumes.importCancelled'))
      queryClient.invalidateQueries({ queryKey: ['project-volumes', projectId] })
      queryClient.invalidateQueries({ queryKey: ['project-volume-transfers', projectId] })
    }
    catch (error) {
      toast.error(error instanceof Error ? error.message : t('errors.request.failed'))
    }
    finally {
      setActiveTransferId(null)
      setStage('idle')
      setProgress(0)
    }
  }

  const busy = stage !== 'idle'
  const statusText = stage === 'hashing'
    ? t('projectVolumes.hashing', { percent: Math.round(progress) })
    : stage === 'preparing'
      ? t('projectVolumes.preparingTransfer')
      : stage === 'uploading'
        ? t('projectVolumes.uploading', { percent: Math.round(progress) })
        : stage === 'verifying'
          ? t('projectVolumes.verifyingImport')
          : stage === 'cancelling'
            ? t('projectVolumes.cancellingTransfer')
            : ''

  return (
    <Dialog open={open} onOpenChange={next => !busy && onOpenChange(next)}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('projectVolumes.import')}</DialogTitle>
          <DialogDescription>{t('projectVolumes.importDescription')}</DialogDescription>
        </DialogHeader>
        <Alert>
          <Upload />
          <AlertTitle>{t('projectVolumes.directTransfer')}</AlertTitle>
          <AlertDescription>{t('projectVolumes.importNoResume')}</AlertDescription>
        </Alert>
        <form className="grid gap-4" onSubmit={form.handleSubmit(startImport)}>
          <div className="grid gap-4 md:grid-cols-2">
            <Field error={form.formState.errors.displayName?.message} label={t('projectVolumes.name')} required>
              <Input {...form.register('displayName')} placeholder={t('projectVolumes.displayNamePlaceholder')} />
            </Field>
            <Field label={t('projectVolumes.archiveFormat')} required>
              <input type="hidden" {...form.register('format')} />
              <NativeSelect disabled value={importFormat}>
                {importFormat === 'tar_gz'
                  ? <option value="tar_gz">{t('projectVolumes.formatTarGz')}</option>
                  : <option value="raw_zst">{t('projectVolumes.formatRawZst')}</option>}
              </NativeSelect>
            </Field>
            <Field error={form.formState.errors.clusterId?.message} label={t('projectVolumes.cluster')} required>
              <ProjectVolumeClusterSelect
                projectId={projectId}
                value={clusterId}
                onChange={(value) => {
                  form.setValue('clusterId', value, { shouldDirty: true, shouldValidate: true })
                  form.setValue('storageClassName', '')
                }}
              />
            </Field>
            <Field error={form.formState.errors.storageClassName?.message} label={t('projectVolumes.storageClass')} required>
              <ProjectVolumeStorageClassSelect
                clusterId={clusterId}
                projectId={projectId}
                value={form.watch('storageClassName')}
                onChange={value => form.setValue('storageClassName', value, { shouldDirty: true, shouldValidate: true })}
              />
            </Field>
            <Field error={form.formState.errors.capacity?.message} label={t('projectVolumes.capacity')} required>
              <Input {...form.register('capacity')} placeholder={t('projectVolumes.capacityPlaceholder')} />
            </Field>
            <Field label={t('projectVolumes.accessMode')} required>
              <NativeSelect {...form.register('accessMode')}>
                <option value="ReadWriteOnce">ReadWriteOnce</option>
                <option value="ReadWriteOncePod">ReadWriteOncePod</option>
                <option value="ReadOnlyMany">ReadOnlyMany</option>
                <option value="ReadWriteMany">ReadWriteMany</option>
              </NativeSelect>
            </Field>
            <Field label={t('projectVolumes.volumeMode')} required>
              <input type="hidden" {...form.register('volumeMode')} />
              <NativeSelect
                value={volumeMode}
                onChange={(event) => {
                  const nextMode = event.target.value as ProjectVolumeMode
                  form.setValue('volumeMode', nextMode, { shouldDirty: true, shouldValidate: true })
                  form.setValue('format', nextMode === 'Block' ? 'raw_zst' : 'tar_gz', { shouldDirty: true, shouldValidate: true })
                }}
              >
                <option value="Filesystem">Filesystem</option>
                <option value="Block">Block</option>
              </NativeSelect>
            </Field>
          </div>
          <Field label={t('projectVolumes.chooseFile')} required>
            <Input
              accept=".tar.gz,.tgz,.raw.zst,.zst,application/gzip,application/zstd"
              disabled={busy}
              type="file"
              onChange={event => setFile(event.target.files?.[0] ?? null)}
            />
            {file && <p className="mt-1 text-xs text-muted-foreground">{t('projectVolumes.selectedFile', { name: file.name, size: formatBytes(file.size) })}</p>}
          </Field>
          {statusText && (
            <div className="grid gap-2" aria-live="polite">
              <div className="h-2 overflow-hidden rounded-full bg-muted">
                <div className="h-full bg-primary transition-[width]" style={{ width: `${Math.min(100, progress)}%` }} />
              </div>
              <p className="text-sm text-muted-foreground">{statusText}</p>
            </div>
          )}
          <DialogFooter>
            {!busy && (
              <Button disabled={!file || !form.formState.isValid} type="submit">
                <Upload className="size-4" />
                {t('projectVolumes.importArchive')}
              </Button>
            )}
            {busy && (
              <Button disabled={stage === 'cancelling'} type="button" variant="destructive" onClick={cancelCurrentImport}>
                {t('projectVolumes.cancelTransfer')}
              </Button>
            )}
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
