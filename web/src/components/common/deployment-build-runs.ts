import type { BuildRun } from '@/api'

export function buildRunImageRef(run: BuildRun) {
  if (run.imageRef)
    return run.imageRef
  if (run.targetRepository)
    return `${run.targetRepository}:${run.targetTag || 'latest'}`
  return ''
}

export function latestDeployableBuildRuns(runs: BuildRun[]) {
  const ordered = runs
    .filter(run => run.status === 'succeeded' && Boolean(run.deploymentTargetId?.trim()) && Boolean(buildRunImageRef(run)))
    .sort((left, right) => buildRunTime(right) - buildRunTime(left))
  const seen = new Set<string>()
  const output: BuildRun[] = []
  for (const run of ordered) {
    const image = buildRunImageRef(run).trim()
    if (!image || seen.has(image))
      continue
    seen.add(image)
    output.push(run)
  }
  return output
}

export function buildRunOptionLabel(run: BuildRun) {
  const branch = run.sourceBranch || run.sourceTag || '-'
  const image = buildRunImageRef(run) || run.targetRepository || run.id
  const digest = shortImageDigest(run.imageDigest)
  return [branch, image, digest].filter(Boolean).join(' · ')
}

export function shortImageDigest(digest: string) {
  const normalized = digest.trim()
  if (!normalized)
    return ''
  if (normalized.startsWith('sha256:'))
    return `sha256:${normalized.slice('sha256:'.length, 'sha256:'.length + 12)}`
  return normalized.length > 18 ? `${normalized.slice(0, 18)}...` : normalized
}

function buildRunTime(run: BuildRun) {
  return new Date(run.finishedAt || run.createdAt || 0).getTime()
}
