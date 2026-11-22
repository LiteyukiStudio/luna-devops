import type { BillingPeriodSelection } from './billing-page-utils'
import type { Project, User } from '@/api'
import { Plus, RotateCw, X } from 'lucide-react'
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
  projectsError = false,
  projectsLoading,
  selectedProjectIds,
  userItems,
  usersError = false,
  usersLoading,
  usersRetrying = false,
  visibleProjectItems,
  onBillingUserChange,
  onCreateTransaction,
  onPeriodChange,
  onProjectFilterChange,
  onRetryUsers,
}: {
  billingPeriod: BillingPeriodSelection
  billingUserScopeId: string
  canManageBilling: boolean
  projectsError?: boolean
  projectsLoading: boolean
  selectedProjectIds: string[]
  userItems: User[]
  usersError?: boolean
  usersLoading: boolean
  usersRetrying?: boolean
  visibleProjectItems: Project[]
  onBillingUserChange: (userId: string) => void
  onCreateTransaction: () => void
  onPeriodChange: (period: BillingPeriodSelection) => void
  onProjectFilterChange: (projectIds: string[]) => void
  onRetryUsers?: () => void
}) {
  const { t } = useTranslation()

  return (
    <div className="flex min-w-0 flex-col gap-2 @4xl/workspace:flex-row @4xl/workspace:flex-wrap @4xl/workspace:items-center">
      <BillingPeriodPicker period={billingPeriod} onChange={onPeriodChange} />
      {canManageBilling && (
        <div className="flex w-full items-center gap-2 @4xl/workspace:w-auto">
          <UserSelect
            allLabel={t('billingPage.allUsers')}
            ariaLabel={t('billingPage.billingUserScope')}
            className="min-w-0 flex-1 @4xl/workspace:w-64"
            emptyLabel={t('billingPage.emptyUsers')}
            error={usersError}
            includeAll
            loading={usersLoading}
            placeholder={t('billingPage.selectBillingUser')}
            users={userItems}
            value={billingUserScopeId}
            onChange={onBillingUserChange}
          />
          {!usersLoading && usersError && onRetryUsers && (
            <Button
              className="shrink-0"
              disabled={usersRetrying}
              size="sm"
              type="button"
              variant="outline"
              onClick={onRetryUsers}
            >
              <RotateCw className={usersRetrying ? 'size-4 motion-safe:animate-spin' : 'size-4'} />
              {t('common.retry')}
            </Button>
          )}
        </div>
      )}
      <div className="w-full @4xl/workspace:w-64">
        <ProjectSpaceMultiSelect
          error={projectsError}
          loading={projectsLoading}
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
            disabled={usersLoading || (userItems.length === 0 && !usersError)}
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
