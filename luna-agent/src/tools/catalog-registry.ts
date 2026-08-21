import { ToolCatalog } from "./catalog.js"

export type ToolCatalogRefresh = {
  changed: boolean
  previousDigest: string
  currentDigest: string
  version: string
}

/** Builds a complete immutable catalog before atomically publishing it. */
export class ToolCatalogRegistry {
  private readonly snapshots = new Map<string, ToolCatalog>()
  private currentCatalog: ToolCatalog
  private currentVersion: string

  constructor(catalog: ToolCatalog, version = "startup") {
    this.currentCatalog = catalog
    this.currentVersion = version
    this.snapshots.set(catalog.digest, catalog)
  }

  current(): ToolCatalog { return this.currentCatalog }
  digest(): string { return this.currentCatalog.digest }
  version(): string { return this.currentVersion }

  get(digest: string): ToolCatalog {
    const catalog = this.snapshots.get(digest)
    if (!catalog) throw new Error("ai.tool_catalog_snapshot_unavailable")
    return catalog
  }

  refresh(input: unknown, version: string): ToolCatalogRefresh {
    if (version === this.currentVersion) {
      return {
        changed: false,
        previousDigest: this.currentCatalog.digest,
        currentDigest: this.currentCatalog.digest,
        version,
      }
    }
    const prepared = ToolCatalog.load(input)
    const previousDigest = this.currentCatalog.digest
    this.snapshots.set(prepared.digest, prepared)
    this.currentCatalog = prepared
    this.currentVersion = version
    return { changed: previousDigest !== prepared.digest, previousDigest, currentDigest: prepared.digest, version }
  }

  retain(activeDigests: Iterable<string>): void {
    const retained = new Set(activeDigests)
    retained.add(this.currentCatalog.digest)
    for (const digest of this.snapshots.keys()) {
      if (!retained.has(digest)) this.snapshots.delete(digest)
    }
  }
}
