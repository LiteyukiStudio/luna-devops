const DOCKER_HUB_HOSTS = new Set([
  'docker.io',
  'index.docker.io',
  'registry-1.docker.io',
])

export interface FamiliarImageRefParts {
  /** 遵循 Docker / OCI Familiar 规范化后的完整镜像引用（用于 title 提示或复制）。 */
  full: string
  /** 去除末尾 :tag / @digest 后的镜像仓库名（非 Docker Hub 保留域名，Docker Hub 官方镜像省略 docker.io/library/）。 */
  repository: string
  /** 末尾的 :tag 或简短 @digest，供窄容器在仓库名过长时中间截断并完整保留尾部标签（如 thisisanlong…:latest）。 */
  suffix: string
}

/**
 * 解析并规范化容器镜像引用（遵循 Docker `distribution/reference` Familiar 规则）：
 * - `docker.io/library/memcached:1.6-alpine` -> `memcached:1.6-alpine`
 * - `docker.io/typesense/typesense:28.0` -> `typesense/typesense:28.0`
 * - `ghcr.io/usememos/memos:stable` -> `ghcr.io/usememos/memos:stable`（非 Docker Hub 保留域名）
 * - 未显式携带 tag 或 digest 时补齐 `fallbackTag` 或 `:latest`。
 */
export function parseFamiliarImageRef(imageRef: string, fallbackTag = ''): FamiliarImageRefParts | null {
  const trimmed = imageRef.trim()
  if (!trimmed)
    return null

  const atIndex = trimmed.indexOf('@')
  const withoutDigest = atIndex >= 0 ? trimmed.slice(0, atIndex) : trimmed
  const digest = atIndex >= 0 ? trimmed.slice(atIndex + 1).trim() : ''

  const lastSlash = withoutDigest.lastIndexOf('/')
  const lastColon = withoutDigest.lastIndexOf(':')
  const hasTag = lastColon > lastSlash
  const rawName = (hasTag ? withoutDigest.slice(0, lastColon) : withoutDigest).trim()
  const explicitTag = hasTag ? withoutDigest.slice(lastColon + 1).trim() : ''
  if (!rawName)
    return null

  const segments = rawName.split('/').filter(Boolean)
  if (segments.length === 0)
    return null

  const first = segments[0]!
  const firstLower = first.toLowerCase()
  const hasExplicitDomain = segments.length > 1
    && (DOCKER_HUB_HOSTS.has(firstLower) || first.includes('.') || first.includes(':') || firstLower === 'localhost')

  let repository: string
  if (hasExplicitDomain && DOCKER_HUB_HOSTS.has(firstLower)) {
    const repoSegments = segments.slice(1)
    repository = repoSegments.length === 2 && repoSegments[0] === 'library'
      ? repoSegments[1]!
      : repoSegments.join('/')
  }
  else if (!hasExplicitDomain && segments.length === 2 && segments[0] === 'library') {
    repository = segments[1]!
  }
  else {
    repository = segments.join('/')
  }

  const normalizedFallback = fallbackTag.trim()
  const tag = explicitTag || (!digest ? (normalizedFallback || 'latest') : '')
  const full = `${repository}${tag ? `:${tag}` : ''}${digest ? `@${digest}` : ''}`

  let suffix = ''
  if (tag) {
    suffix = `:${tag}`
  }
  else if (digest) {
    const colonIdx = digest.indexOf(':')
    const algo = colonIdx > 0 ? digest.slice(0, colonIdx) : ''
    const hex = colonIdx > 0 ? digest.slice(colonIdx + 1) : digest
    const shortDigest = hex.length > 7 ? `${algo ? `${algo}:` : ''}${hex.slice(0, 7)}…` : digest
    suffix = `@${shortDigest}`
  }

  return {
    full,
    repository,
    suffix,
  }
}

/** 紧凑展示镜像引用：保留最后两段仓库路径和 tag，完整值应通过 title 或复制入口提供。 */
export function shortImageRef(imageRef: string) {
  const value = imageRef.trim()
  if (!value)
    return '-'
  const [repository, tag = ''] = value.split(':')
  const parts = repository.split('/').filter(Boolean)
  const compactRepository = parts.length > 2 ? `${parts.at(-2)}/${parts.at(-1)}` : repository
  return tag ? `${compactRepository}:${tag}` : compactRepository
}
