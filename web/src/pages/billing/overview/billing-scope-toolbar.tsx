import type { BillingPeriodSelection } from './billing-page-utils'
import type { Project, User } from '@/api'
import { Plus, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { PageChromeTools } from '@/components/common/page-chrome'
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
    <div className="flex min-w-0 flex-col gap-2 @4xl/workspace:flex-row @4xl/workspace:flex-wrap @4xl/workspace:items-center">
      <BillingPeriodPicker period={billingPeriod} onChange={onPeriodChange} />
      {canManageBilling && (
        <div className="w-full @4xl/workspace:w-64">
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
      <div className="w-full @4xl/workspace:w-64">
        <ProjectSpaceMultiSelect
          disabled={projectsLoading}
          projects={visibleProjectItems}
          value={selectedProjectIds}
          onChange={onProjectFilterChange}
        />
      </div>
      {selectedProjectIds.length > 0 && (
        <Button className="shrink-0 self-start @4xl/workspace:self-auto" type="button" variant="ghost" onClick={() => onProjectFilterChange([])}>
          <X />
          {t('billingPage.clearProjectFilter')}
        </Button>
      )}
      {canManageBilling && (
        <PageChromeTools>
          <Button
            disabled={usersLoading || userItems.length === 0}
            type="button"
            onClick={onCreateTransaction}
          >
            <Plus />
            {t('billingPage.createWalletTransaction')}
          </Button>
        </PageChromeTools>
      )}
    </div>
  )
}
