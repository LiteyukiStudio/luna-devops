import type { ModelToolRegistry } from "../../src/provider/provider.js"

export function testRegistry(overrides: Partial<ModelToolRegistry> = {}): ModelToolRegistry {
  return {
    resolve: () => [],
    search: (input, _pageContext, _signal, toolCatalogDigest) => ({
      query: input.query?.trim() ?? "",
      items: [],
      page: input.page ?? 1,
      pageSize: input.pageSize ?? 20,
      total: 0,
      totalPages: 0,
      loadedOperationIds: [],
      missingOperationIds: [],
      catalogDigest: toolCatalogDigest ?? "",
      duplicate: false,
      cacheHit: false,
    }),
    details: (operationIds, toolCatalogDigest) => ({
      items: [],
      loadedOperationIds: [],
      alreadySelectedOperationIds: [],
      missingOperationIds: operationIds,
      catalogDigest: toolCatalogDigest ?? "",
      duplicate: false,
      cacheHit: false,
    }),
    ...overrides,
  }
}
