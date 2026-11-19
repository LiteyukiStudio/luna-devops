import type { ToolCatalog } from "./catalog.js"

export type ToolCatalogRefresh = {
  changed: boolean
  previousDigest: string
  currentDigest: string
  tenantDigest: string
  version: string
}

/**
 * Maintains immutable deterministic role-isolated snapshots (`tenant` vs `admin`)
 * so tenant sessions never see `adminOnly` operations while both roles retain
 * stable Prompt KV Cache digests across turns.
 */
export class ToolCatalogRegistry {
  private readonly snapshots = new Map<string, ToolCatalog>()
  private currentAdminCatalog: ToolCatalog
  private currentTenantCatalog: ToolCatalog
  private currentVersion: string

  constructor(catalog: ToolCatalog, version = "startup") {
    this.currentAdminCatalog = catalog
    this.currentTenantCatalog = catalog.withoutAdminOperations()
    this.currentVersion = version
    this.snapshots.set(this.currentAdminCatalog.digest, this.currentAdminCatalog)
    this.snapshots.set(this.currentTenantCatalog.digest, this.currentTenantCatalog)
  }

  current(): ToolCatalog { return this.currentAdminCatalog }
  tenant(): ToolCatalog { return this.currentTenantCatalog }
  forRole(platformRole: string | undefined): ToolCatalog {
    return platformRole === "admin" ? this.currentAdminCatalog : this.currentTenantCatalog
  }
  digest(): string { return this.currentAdminCatalog.digest }
  digestForRole(platformRole: string | undefined): string {
    return this.forRole(platformRole).digest
  }
  version(): string { return this.currentVersion }
  has(digest: string): boolean { return this.snapshots.has(digest) }

  get(digest: string): ToolCatalog {
    const catalog = this.snapshots.get(digest)
    if (!catalog) throw new Error("ai.tool_catalog_snapshot_unavailable")
    return catalog
  }

  refresh(prepared: ToolCatalog, version: string): ToolCatalogRefresh {
    if (version === this.currentVersion) {
      return {
        changed: false,
        previousDigest: this.currentAdminCatalog.digest,
        currentDigest: this.currentAdminCatalog.digest,
        tenantDigest: this.currentTenantCatalog.digest,
        version,
      }
    }
    const previousDigest = this.currentAdminCatalog.digest
    const preparedTenant = prepared.withoutAdminOperations()
    this.snapshots.set(prepared.digest, prepared)
    this.snapshots.set(preparedTenant.digest, preparedTenant)
    this.currentAdminCatalog = prepared
    this.currentTenantCatalog = preparedTenant
    this.currentVersion = version
    return {
      changed: previousDigest !== prepared.digest,
      previousDigest,
      currentDigest: prepared.digest,
      tenantDigest: preparedTenant.digest,
      version,
    }
  }

  retain(activeDigests: Iterable<string>): void {
    const retained = new Set(activeDigests)
    retained.add(this.currentAdminCatalog.digest)
    retained.add(this.currentTenantCatalog.digest)
    for (const digest of this.snapshots.keys()) {
      if (!retained.has(digest)) this.snapshots.delete(digest)
    }
  }
}
