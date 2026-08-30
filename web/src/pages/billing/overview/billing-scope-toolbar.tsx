import type { BillingPeriodSelection } from './billing-page-utils'
import type { Project, User } from '@/api'
import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ProjectSpaceMultiSelect } from '@/components/common/project-space-select'
import { UserSelect } from '@/components/common/user-select'
import { Button } from '@/components/ui/button'
import { BillingPeriodPicker } from './billing-period-picker'

export function BillingScopeToolbar({
  billingPeriod,
  billingUserScopeId,
  canManageBilling,
  projectsLoading,
  selectedProjectIds,
  userItems,
  usersLoading,
  visibleProjectItems,
  onBillingUserChange,
  onCreateTransaction,
  onPeriodChange,
  onProjectFilterChange,
}: {
  billingPeriod: BillingPeriodSelection
  billingUserScopeId: string
  canManageBilling: boolean
  projectsLoading: boolean
  selectedProjectIds: string[]
  userItems: User[]
  usersLoading: boolean
  visibleProjectItems: Project[]
  onBillingUserChange: (userId: string) => void
  onCreateTransaction: () => void
  onPeriodChange: (period: BillingPeriodSelection) => void
  onProjectFilterChange: (projectIds: string[]) => void
}) {
  const { t } = useTranslation()

  return (
    <div className="flex min-w-0 flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
      <div className="flex min-w-0 flex-1 flex-col gap-2 xl:flex-row xl:items-center">
        <BillingPeriodPicker period={billingPeriod} onChange={onPeriodChange} />
        {canManageBilling && (
          <div className="w-full sm:w-80">
            <UserSelect
              allLabel={t('billingPage.allUsers')}
              ariaLabel={t('billingPage.billingUserScope')}
              disabled={usersLoading}
              emptyLabel={t('billingPage.emptyUsers')}
              includeAll
              placeholder={t('billingPage.selectBillingUser')}
              users={userItems}
              value={billingUserScopeId}
              onChange={onBillingUserChange}
            />
          </div>
        )}
        <div className="w-full sm:w-80">
          <ProjectSpaceMultiSelect
            disabled={projectsLoading}
            projects={visibleProjectItems}
            value={selectedProjectIds}
            onChange={onProjectFilterChange}
          />
        </div>
        {selectedProjectIds.length > 0 && (
          <Button className="h-11 shrink-0 rounded-2xl" type="button" variant="outline" onClick={() => onProjectFilterChange([])}>
            {t('billingPage.clearProjectFilter')}
          </Button>
        )}
      </div>
      {canManageBilling && (
        <Button
          className="h-11 shrink-0 rounded-2xl"
          disabled={usersLoading || userItems.length === 0}
          type="button"
          onClick={onCreateTransaction}
        >
          <Plus size={16} />
          {t('billingPage.createWalletTransaction')}
        </Button>
      )}
    </div>
  )
}
