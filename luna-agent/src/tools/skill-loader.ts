import { readFileSync } from "node:fs"
import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"

export const domainSkillIds = [
  "workspace-iam",
  "source-registry-build",
  "application-deployment",
  "runtime-volumes",
  "gateway-topology",
  "diagnostics-governance",
] as const

export type DomainSkillId = (typeof domainSkillIds)[number]

export type DomainSkillEntry = {
  skillId: DomainSkillId
  name: string
  description: string
  instructions: string
}

export const loadSkillInput = z.object({
  skillIds: z.array(z.enum(domainSkillIds)).min(1).max(3),
}).strict()

export type LoadSkillInput = z.infer<typeof loadSkillInput>

const parsedSkills = loadBundledDomainSkills()

export const loadSkillTool: ModelToolDefinition = {
  operationId: "load_skill",
  description: "按需加载一到三个 Luna DevOps 业务领域的深度工作流规程（如项目空间与权限、代码源与构建、应用与部署、运行时与数据卷、访问入口与拓扑、跨领域排障与平台治理）。同一 Run 内已加载过的领域会自动去重返回缓存标记，不重复占用上下文。仅提供领域执行约束与诊断顺序，不执行平台操作，也不能覆盖系统安全不变量。",
  inputSchema: jsonSchema(loadSkillInput),
}

export type LoadedSkillResultItem = {
  skillId: DomainSkillId
  description: string
  duplicate: boolean
  instructions?: string
}

export type LoadSkillResult = {
  items: LoadedSkillResultItem[]
  loadedSkillIds: DomainSkillId[]
  alreadyLoadedSkillIds: DomainSkillId[]
  duplicate: boolean
  cacheHit: boolean
}

export function listDomainSkills(): ReadonlyArray<Pick<DomainSkillEntry, "skillId" | "name" | "description">> {
  return domainSkillIds.map((skillId) => {
    const entry = parsedSkills.get(skillId)!
    return {
      skillId: entry.skillId,
      name: entry.name,
      description: entry.description,
    }
  })
}

export function loadDomainSkills(
  requestedIds: ReadonlyArray<DomainSkillId>,
  alreadyLoaded: ReadonlySet<string>,
): LoadSkillResult {
  const uniqueRequested: DomainSkillId[] = []
  for (const id of requestedIds) {
    if (!uniqueRequested.includes(id)) uniqueRequested.push(id)
  }
  const items: LoadedSkillResultItem[] = []
  const newlyLoaded: DomainSkillId[] = []
  const skippedAlreadyLoaded: DomainSkillId[] = []

  for (const skillId of uniqueRequested) {
    const entry = parsedSkills.get(skillId)
    if (!entry) throw new Error(`ai.skill_not_found:${skillId}`)
    if (alreadyLoaded.has(skillId)) {
      skippedAlreadyLoaded.push(skillId)
      items.push({
        skillId,
        description: entry.description,
        duplicate: true,
      })
      continue
    }
    newlyLoaded.push(skillId)
    items.push({
      skillId,
      description: entry.description,
      duplicate: false,
      instructions: entry.instructions,
    })
  }

  const allCached = newlyLoaded.length === 0 && skippedAlreadyLoaded.length > 0
  return {
    items,
    loadedSkillIds: newlyLoaded,
    alreadyLoadedSkillIds: skippedAlreadyLoaded,
    duplicate: allCached,
    cacheHit: allCached,
  }
}

function loadBundledDomainSkills(): ReadonlyMap<DomainSkillId, DomainSkillEntry> {
  const entries = new Map<DomainSkillId, DomainSkillEntry>()
  for (const skillId of domainSkillIds) {
    const raw = readFileSync(new URL(`../../skills/domains/${skillId}/SKILL.md`, import.meta.url), "utf8").trim()
    const parsed = parseSkillMarkdown(skillId, raw)
    entries.set(skillId, parsed)
  }
  return entries
}

function parseSkillMarkdown(expectedId: DomainSkillId, raw: string): DomainSkillEntry {
  const match = /^---\r?\n([\s\S]*?)\r?\n---\r?\n+([\s\S]+)$/.exec(raw)
  if (!match) {
    throw new Error(`Invalid frontmatter in domain skill ${expectedId}`)
  }
  const frontmatter = match[1] ?? ""
  const body = (match[2] ?? "").trim()
  let name = ""
  let description = ""
  for (const line of frontmatter.split(/\r?\n/)) {
    const separator = line.indexOf(":")
    if (separator <= 0) continue
    const key = line.slice(0, separator).trim()
    const value = line.slice(separator + 1).trim()
    if (key === "name") name = value
    if (key === "description") description = value
  }
  if (name !== expectedId || !description || !body) {
    throw new Error(`Malformed metadata in domain skill ${expectedId}`)
  }
  return {
    skillId: expectedId,
    name,
    description,
    instructions: body,
  }
}

function jsonSchema(schema: z.ZodType): Record<string, unknown> {
  const value = z.toJSONSchema(schema, { io: "input" }) as Record<string, unknown>
  delete value.$schema
  return value
}
