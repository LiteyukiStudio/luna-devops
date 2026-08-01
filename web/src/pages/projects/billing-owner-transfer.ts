import type { ProjectMember } from '@/api'

export function eligibleBillingOwnerTransferMembers(members: ProjectMember[], billingOwnerUserId: string) {
  return members.filter(member => member.userId !== billingOwnerUserId)
}
