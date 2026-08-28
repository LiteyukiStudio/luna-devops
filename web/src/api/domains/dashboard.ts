import type { DashboardOverview, ResultVisibility } from '../types'
import { request } from '../core'

export const dashboardApi = {
  getDashboard: (visibility?: ResultVisibility) => {
    const query = visibility ? `?visibility=${visibility}` : ''
    return request<DashboardOverview>(`/dashboard${query}`)
  },
}
