import type { User } from '@/api'
import type { useBillingDisplay } from '@/lib/billing-display'
import { useTranslation } from 'react-i18next'
import { FormField as Field } from '@/components/common/form-field'
import { UserSelect } from '@/components/common/user-select'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'

export function BillingWalletTransactionDialog({
  amount,
  billingDisplay,
  description,
  isPending,
  open,
  type,
  userId,
  userItems,
  usersLoading,
  onAmountChange,
  onConfirm,
  onDescriptionChange,
  onOpenChange,
  onTypeChange,
  onUserIdChange,
}: {
  amount: string
  billingDisplay: ReturnType<typeof useBillingDisplay>
  description: string
  isPending: boolean
  open: boolean
  type: 'credit' | 'adjustment'
  userId: string
  userItems: User[]
  usersLoading: boolean
  onAmountChange: (value: string) => void
  onConfirm: () => void
  onDescriptionChange: (value: string) => void
  onOpenChange: (open: boolean) => void
  onTypeChange: (value: 'credit' | 'adjustment') => void
  onUserIdChange: (value: string) => void
}) {
  const { t } = useTranslation()

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('billingPage.walletTransactionTitle')}</DialogTitle>
          <DialogDescription>{t('billingPage.walletTransactionDescription')}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-2">
          <Field label={t('billingPage.user')}>
            <UserSelect
              disabled={usersLoading}
              emptyLabel={t('billingPage.emptyUsers')}
              placeholder={t('billingPage.selectUser')}
              users={userItems}
              value={userId}
              onChange={onUserIdChange}
            />
          </Field>
          <Field hint={t('billingPage.walletTransactionTypeHint')} label={t('billingPage.walletTransactionType')}>
            <Select value={type} onValueChange={value => onTypeChange(value as 'credit' | 'adjustment')}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="credit">{t('billingPage.walletTransactionTypes.credit')}</SelectItem>
                <SelectItem value="adjustment">{t('billingPage.walletTransactionTypes.adjustment')}</SelectItem>
              </SelectContent>
            </Select>
          </Field>
          <Field hint={t('billingPage.walletTransactionAmountHint', { unit: billingDisplay.currencyUnit })} label={t('billingPage.amount')}>
            <Input
              inputMode="decimal"
              placeholder={t('billingPage.walletTransactionAmountPlaceholder')}
              value={amount}
              onChange={event => onAmountChange(event.target.value)}
            />
          </Field>
          <Field label={t('billingPage.descriptionLabel')}>
            <Textarea
              className="min-h-24"
              placeholder={t('billingPage.walletTransactionDescriptionPlaceholder')}
              value={description}
              onChange={event => onDescriptionChange(event.target.value)}
            />
          </Field>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button
            disabled={isPending || !userId || !amount.trim()}
            type="button"
            onClick={onConfirm}
          >
            {t('common.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
