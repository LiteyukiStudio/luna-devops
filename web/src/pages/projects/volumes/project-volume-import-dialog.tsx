import type { ProjectVolumeAccessMode, ProjectVolumeMode, VolumeImportResumeRecord, VolumeTransferFormat } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Pause, Play, Upload } from 'lucide-react'
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
import {
  clearVolumeImportResumeRecord,
  fileMatchesResumeRecord,
  readVolumeImportResumeRecord,
  sha256File,
  uploadVolumeImport,
  writeVolumeImportResumeRecord,
} from './volume-transfer-upload'

interface ImportValues {
  accessMode: ProjectVolumeAccessMode
  capacity: string
  clusterId: string
  displayName: string
  format: VolumeTransferFormat
  storageClassName: string
  volumeMode: ProjectVolumeMode
}

type ImportStage = 'hashing' | 'idle' | 'paused' | 'processing' | 'uploading'

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
  const [resumeRecord, setResumeRecord] = useState<VolumeImportResumeRecord | null>(() => readVolumeImportResumeRecord(projectId))
  const abortRef = useRef<AbortController | null>(null)
  const form = useForm<ImportValues>({ defaultValues: defaults, mode: 'onChange', resolver: zodResolver(importSchema(t('common.required'), t('projectVolumes.nameTooLong'))) })
  const clusterId = form.watch('clusterId')
  const volumeMode = form.watch('volumeMode')
  const importFormat: VolumeTransferFormat = volumeMode === 'Block' ? 'raw_zst' : 'tar_gz'

  const cancelTransfer = useMutation({
    mutationFn: (record: VolumeImportResumeRecord) => api.cancelVolumeTransfer(projectId, record.transferId),
    onSuccess: () => {
      clearVolumeImportResumeRecord(projectId)
      setResumeRecord(null)
      setFile(null)
      setStage('idle')
      toast.success(t('projectVolumes.importCancelled'))
      queryClient.invalidateQueries({ queryKey: ['project-volumes', projectId] })
    },
    onError: error => toast.error(error.message),
  })

  async function finishUpload(selectedFile: File, record: VolumeImportResumeRecord) {
    const controller = new AbortController()
    abortRef.current = controller
    setStage('uploading')
    try {
      const transfer = await api.getVolumeTransfer(projectId, record.transferId)
      await uploadVolumeImport({
        file: selectedFile,
        projectId,
        sha256: record.sha256,
        signal: controller.signal,
        transfer,
        onProgress: current => setProgress(current.percent),
      })
      setStage('processing')
      clearVolumeImportResumeRecord(projectId, record.transferId)
      setResumeRecord(null)
      toast.success(t('projectVolumes.importQueued'))
      queryClient.invalidateQueries({ queryKey: ['project-volumes', projectId] })
      queryClient.invalidateQueries({ queryKey: ['project-volume-transfers', projectId] })
      onOpenChange(false)
      form.reset(defaults)
      setFile(null)
      setProgress(0)
      setStage('idle')
    }
    catch (error) {
      if (controller.signal.aborted) {
        setStage('paused')
        toast.message(t('projectVolumes.paused'))
      }
      else {
        setStage('idle')
        toast.error(error instanceof Error ? error.message : t('errors.request.failed'))
      }
    }
    finally {
      if (abortRef.current === controller)
        abortRef.current = null
    }
  }

  async function startImport(values: ImportValues) {
    if (!file) {
      toast.error(t('projectVolumes.fileRequired'))
      return
    }
    if (file.size === 0) {
      toast.error(t('projectVolumes.fileEmpty'))
      return
    }
    try {
      setStage('hashing')
      setProgress(0)
      const sha256 = await sha256File(file, (processed, total) => setProgress(total > 0 ? (processed / total) * 100 : 100))
      const result = await api.createVolumeImport(projectId, {
        ...values,
        displayName: values.displayName.trim(),
        capacity: values.capacity.trim(),
        filename: file.name,
        contentLength: file.size,
        sha256,
      })
      const record: VolumeImportResumeRecord = {
        projectId,
        transferId: result.transfer.id,
        volumeId: result.volume.id,
        filename: file.name,
        size: file.size,
        lastModified: file.lastModified,
        sha256,
        format: values.format,
        createdAt: new Date().toISOString(),
      }
      writeVolumeImportResumeRecord(record)
      setResumeRecord(record)
      await finishUpload(file, record)
    }
    catch (error) {
      setStage('idle')
      toast.error(error instanceof Error ? error.message : t('errors.request.failed'))
    }
  }

  async function resumeImport() {
    if (!resumeRecord || !file) {
      toast.error(t('projectVolumes.fileRequired'))
      return
    }
    if (!fileMatchesResumeRecord(file, resumeRecord)) {
      toast.error(t('projectVolumes.resumeFileMismatch'))
      return
    }
    await finishUpload(file, resumeRecord)
  }

  const busy = stage === 'hashing' || stage === 'uploading' || stage === 'processing'
  const statusText = stage === 'hashing'
    ? t('projectVolumes.hashing', { percent: Math.round(progress) })
    : stage === 'uploading'
      ? t('projectVolumes.uploading', { percent: Math.round(progress) })
      : stage === 'processing'
        ? t('projectVolumes.processing')
        : stage === 'paused'
          ? t('projectVolumes.paused')
          : ''

  return (
    <Dialog open={open} onOpenChange={next => !busy && onOpenChange(next)}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('projectVolumes.import')}</DialogTitle>
          <DialogDescription>{t('projectVolumes.importDescription')}</DialogDescription>
        </DialogHeader>
        {resumeRecord && (
          <Alert>
            <Upload />
            <AlertTitle>{t('projectVolumes.resumeAvailable', { filename: resumeRecord.filename })}</AlertTitle>
            <AlertDescription>{t('projectVolumes.resumeChooseSameFile')}</AlertDescription>
          </Alert>
        )}
        <form className="grid gap-4" onSubmit={form.handleSubmit(startImport)}>
          {!resumeRecord && (
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
          )}
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
            {resumeRecord && !busy && (
              <Button
                type="button"
                variant="ghost"
                onClick={() => {
                  clearVolumeImportResumeRecord(projectId, resumeRecord.transferId)
                  setResumeRecord(null)
                  setFile(null)
                }}
              >
                {t('projectVolumes.discardResume')}
              </Button>
            )}
            {resumeRecord && !busy && (
              <Button disabled={!file} type="button" onClick={resumeImport}>
                <Play className="size-4" />
                {t('projectVolumes.resume')}
              </Button>
            )}
            {!resumeRecord && !busy && (
              <Button disabled={!file || !form.formState.isValid} type="submit">
                <Upload className="size-4" />
                {t('projectVolumes.importArchive')}
              </Button>
            )}
            {stage === 'uploading' && (
              <Button type="button" variant="outline" onClick={() => abortRef.current?.abort()}>
                <Pause className="size-4" />
                {t('projectVolumes.pause')}
              </Button>
            )}
            {resumeRecord && !busy && (
              <Button disabled={cancelTransfer.isPending} type="button" variant="destructive" onClick={() => cancelTransfer.mutate(resumeRecord)}>
                {t('projectVolumes.cancelTransfer')}
              </Button>
            )}
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
