import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Upload } from 'lucide-react'
import { useRef, useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { ControlledCheckboxField } from '@/components/common/checkbox-field'
import { FormField as Field } from '@/components/common/form-field'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Textarea } from '@/components/ui/textarea'
import { parsePlatformTemplateBundleJson } from './platform-template-form'

function createImportFormSchema(t: ReturnType<typeof useTranslation>['t']) {
  return z.object({
    jsonText: z.string(),
    overwrite: z.boolean(),
  }).superRefine((value, ctx) => {
    const parsed = parsePlatformTemplateBundleJson(value.jsonText)
    if ('errorCode' in parsed) {
      const message = parsed.errorCode === 'jsonEmpty'
        ? t('appTemplatesPage.platformAdmin.validation.jsonEmpty')
        : parsed.errorCode === 'jsonInvalid'
          ? t('appTemplatesPage.platformAdmin.validation.jsonInvalid')
          : t('appTemplatesPage.platformAdmin.validation.bundleInvalid')
      ctx.addIssue({ code: 'custom', message, path: ['jsonText'] })
    }
  })
}

type ImportFormValues = z.infer<ReturnType<typeof createImportFormSchema>>

export function PlatformTemplateImportDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const [importing, setImporting] = useState(false)

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!importing)
          onOpenChange(nextOpen)
      }}
    >
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('appTemplatesPage.platformAdmin.importDialogTitle')}</DialogTitle>
          <DialogDescription>{t('appTemplatesPage.platformAdmin.importDialogDescription')}</DialogDescription>
        </DialogHeader>
        {open && (
          <PlatformTemplateImportForm
            onCancel={() => onOpenChange(false)}
            onImported={() => onOpenChange(false)}
            onPendingChange={setImporting}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

function PlatformTemplateImportForm({
  onCancel,
  onImported,
  onPendingChange,
}: {
  onCancel: () => void
  onImported: () => void
  onPendingChange: (pending: boolean) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [selectedFileName, setSelectedFileName] = useState('')
  const form = useForm<ImportFormValues>({
    defaultValues: { jsonText: '', overwrite: false },
    resolver: zodResolver(createImportFormSchema(t)),
  })
  const { control, formState: { errors }, register, setValue } = form

  const importMutation = useMutation({
    mutationFn: async (values: ImportFormValues) => {
      const parsed = parsePlatformTemplateBundleJson(values.jsonText)
      if ('errorCode' in parsed)
        throw new Error(t('appTemplatesPage.platformAdmin.validation.bundleInvalid'))
      return api.importPlatformAppTemplate({
        bundle: parsed.bundle,
        overwrite: values.overwrite,
      })
    },
    onMutate: () => onPendingChange(true),
    onSettled: () => onPendingChange(false),
    onSuccess: async () => {
      toast.success(t('appTemplatesPage.platformAdmin.importedToast'))
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['app-templates'] }),
        queryClient.invalidateQueries({ queryKey: ['app-template'] }),
        queryClient.invalidateQueries({ queryKey: ['platform-app-templates'] }),
      ])
      onImported()
    },
    onError: error => toast.error(error.message),
  })

  const pending = importMutation.isPending
  const submit = form.handleSubmit(values => importMutation.mutate(values))

  async function handleFileSelect(file: File | undefined) {
    if (!file)
      return
    const text = await file.text()
    setSelectedFileName(file.name)
    setValue('jsonText', text, { shouldDirty: true, shouldValidate: true })
  }

  return (
    <form className="grid gap-4" noValidate onSubmit={submit}>
      <div className="flex flex-wrap items-center gap-3">
        <input
          ref={fileInputRef}
          accept=".json,application/json"
          className="sr-only"
          disabled={pending}
          type="file"
          onChange={(event) => {
            void handleFileSelect(event.target.files?.[0])
            event.target.value = ''
          }}
        />
        <Button
          disabled={pending}
          size="sm"
          type="button"
          variant="outline"
          onClick={() => fileInputRef.current?.click()}
        >
          <Upload className="size-3.5" />
          {t('appTemplatesPage.platformAdmin.selectJsonFile')}
        </Button>
        {selectedFileName && (
          <span className="truncate text-xs text-muted-foreground">
            {t('appTemplatesPage.platformAdmin.selectedFileName', { name: selectedFileName })}
          </span>
        )}
      </div>

      <Field
        error={errors.jsonText?.message}
        label={t('appTemplatesPage.platformAdmin.jsonContent')}
        required
      >
        <Textarea
          className="min-h-48 font-mono text-xs"
          disabled={pending}
          placeholder={t('appTemplatesPage.platformAdmin.jsonPlaceholder')}
          rows={10}
          {...register('jsonText')}
        />
      </Field>

      <Controller
        control={control}
        name="overwrite"
        render={({ field }) => (
          <ControlledCheckboxField
            description={t('appTemplatesPage.platformAdmin.overwriteExistingDescription')}
            disabled={pending}
            field={field}
          >
            {t('appTemplatesPage.platformAdmin.overwriteExisting')}
          </ControlledCheckboxField>
        )}
      />

      <DialogFooter>
        <Button disabled={pending} type="button" variant="outline" onClick={onCancel}>
          {t('common.cancel')}
        </Button>
        <Button disabled={pending} type="submit">
          {pending ? t('appTemplatesPage.platformAdmin.importing') : t('appTemplatesPage.platformAdmin.importSubmit')}
        </Button>
      </DialogFooter>
    </form>
  )
}
