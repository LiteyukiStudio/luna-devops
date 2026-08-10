import type { PaginatedResponse, PaginationParams } from './types'

/**
 * 仅供资源选择器和详情聚合使用的有界首屏请求。
 * 高增长管理列表必须使用各 domain 的 `list*Page` 方法保留完整分页契约。
 */
export const selectionPageParams = {
  page: 1,
  pageSize: 100,
} satisfies PaginationParams

export function selectionItems<T>(response: PaginatedResponse<T>) {
  return response.items
}
