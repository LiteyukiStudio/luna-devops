import type { Project, ProjectMember } from '@/api'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation } from '@tanstack/react-query'
import { ArrowRightLeft, Inbox } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { z } from 'zod'
import { api } from '@/api'
import { FormField as Field } from '@/components/common/form-field'
import { UserAvatar } from '@/components/common/user-avatar'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { eligibleBillingOwnerTransferMembers } from './billing-owner-transfer'

const transferFormSchema = z.object({
  recipientUserId: z.string().min(1),
})

type TransferForm = z.infer<typeof transferFormSchema>

export function BillingOwnerTransferDialog({ members, project }: { members: ProjectMember[], project: Project }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const form = useForm<TransferForm>({
    resolver: zodResolver(transferFormSchema),
    mode: 'onChange',
    defaultValues: { recipientUserId: '' },
  })
  const candidates = useMemo(
    () => eligibleBillingOwnerTransferMembers(members, project.billingOwnerUserId),
    [members, project.billingOwnerUserId],
  )
  const transfer = useMutation({
    mutationFn: (recipientUserId: string) => api.createBillingOwnerTransferRequest(project.id, recipientUserId),
    onSuccess: () => {
      setOpen(false)
      form.reset()
      toast.success(t('projectSpaces.billingOwnerTransfer.requested'), {
        description: t('projectSpaces.billingOwnerTransfer.requestedDescription'),
        action: {
          label: t('projectSpaces.billingOwnerTransfer.openInbox'),
          onClick: () => navigate('/inbox'),
        },
      })
    },
    onError: error => toast.error(error instanceof Error ? error.message : t('projectSpaces.billingOwnerTransfer.requestFailed')),
  })

  const handleOpenChange = (nextOpen: boolean) => {
    setOpen(nextOpen)
    if (!nextOpen && !transfer.isPending)
      form.reset()
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>
        <Button className="shrink-0" size="sm" type="button" variant="outline">
          <ArrowRightLeft className="size-4" />
          {t('projectSpaces.billingOwnerTransfer.trigger')}
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('projectSpaces.billingOwnerTransfer.title')}</DialogTitle>
          <DialogDescription>{t('projectSpaces.billingOwnerTransfer.description', { projectName: project.name })}</DialogDescription>
        </DialogHeader>

        <form className="grid gap-4" onSubmit={form.handleSubmit(values => transfer.mutate(values.recipientUserId))}>
          <div className="grid gap-4 py-2">
            {candidates.length > 0
              ? (
                  <Field
                    required
                    error={form.formState.errors.recipientUserId ? t('projectSpaces.billingOwnerTransfer.recipientRequired') : undefined}
                    hint={t('projectSpaces.billingOwnerTransfer.recipientHint')}
                    label={t('projectSpaces.billingOwnerTransfer.recipient')}
                  >
                    <Select
                      value={form.watch('recipientUserId')}
                      onValueChange={value => form.setValue('recipientUserId', value, { shouldDirty: true, shouldValidate: true })}
                    >
                      <SelectTrigger aria-invalid={Boolean(form.formState.errors.recipientUserId)} className="w-full">
                        <SelectValue placeholder={t('projectSpaces.billingOwnerTransfer.recipientPlaceholder')} />
                      </SelectTrigger>
                      <SelectContent position="popper">
                        {candidates.map(member => (
                          <SelectItem key={member.userId} value={member.userId}>
                            <UserAvatar className="size-6" user={member} />
                            <span className="min-w-0">
                              <span className="block truncate font-medium">{member.name || member.email}</span>
                              <span className="block truncate text-xs text-muted-foreground">{member.email}</span>
                            </span>
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </Field>
                )
              : (
                  <Alert>
                    <Inbox />
                    <AlertTitle>{t('projectSpaces.billingOwnerTransfer.noCandidatesTitle')}</AlertTitle>
                    <AlertDescription>{t('projectSpaces.billingOwnerTransfer.noCandidatesDescription')}</AlertDescription>
                  </Alert>
                )}

            <Alert>
              <ArrowRightLeft />
              <AlertTitle>{t('projectSpaces.billingOwnerTransfer.confirmationTitle')}</AlertTitle>
              <AlertDescription>{t('projectSpaces.billingOwnerTransfer.confirmationDescription')}</AlertDescription>
            </Alert>
          </div>

          <DialogFooter>
            <Button disabled={transfer.isPending} type="button" variant="outline" onClick={() => handleOpenChange(false)}>
              {t('common.cancel')}
            </Button>
            <Button disabled={!form.formState.isValid || transfer.isPending} type="submit">
              {transfer.isPending ? t('common.loading') : t('projectSpaces.billingOwnerTransfer.submit')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
