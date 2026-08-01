import type { ProjectMember } from '@/api'
import { describe, expect, it } from 'vitest'
import { ProjectRole } from '@/lib/roles'
import { eligibleBillingOwnerTransferMembers } from './billing-owner-transfer'

const members: ProjectMember[] = [
  { id: 'member-owner', projectId: 'project-1', userId: 'user-owner', role: ProjectRole.Owner, email: 'owner@example.com', name: 'Owner' },
  { id: 'member-admin', projectId: 'project-1', userId: 'user-admin', role: ProjectRole.Admin, email: 'admin@example.com', name: 'Admin' },
]

describe('eligibleBillingOwnerTransferMembers', () => {
  it('excludes the current billing owner and keeps other existing members', () => {
    expect(eligibleBillingOwnerTransferMembers(members, 'user-owner')).toEqual([members[1]])
  })
})
