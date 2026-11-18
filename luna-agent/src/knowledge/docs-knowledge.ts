import { existsSync, readFileSync } from "node:fs"
import { BM25FIndex } from "../tools/retrieval/bm25.js"
import { UnicodeLexicalTokenizer } from "../tools/retrieval/tokenizer.js"

export type DocsLocale = "zh" | "en"

export type DocsSearchItem = {
  docId: string
  sectionId: string
  title: string
  section: string
  snippet: string
  url?: string
}

export type DocsTreeEntry =
  | { type: "dir", path: string, label: string, documentCount: number, children?: DocsTreeEntry[] }
  | { type: "doc", docId: string, title: string, description: string }

export type DocsListResult = {
  locale: DocsLocale
  path: string
  entries: DocsTreeEntry[]
}

export type DocsReadResult = {
  docId: string
  title: string
  section?: string
  sections: Array<{ sectionId: string, heading: string }>
  content: string
  truncated: boolean
  url?: string
}

type DocsChunk = {
  key: string
  docId: string
  locale: DocsLocale
  sectionId: string
  parentSectionId?: string
  parentHeading?: string
  level: 1 | 2 | 3
  continuation?: boolean
  title: string
  heading: string
  body: string
}

type DocsPage = {
  docId: string
  title: string
  description: string
  chunks: DocsChunk[]
}

type NavigationItem = { type?: string, name?: string, label?: string, text?: string, activeMatch?: string }

type DocsManifest = { digest: string, files: string[], navigation?: string[] }

const maxChunkCharacters = 2_400
const snippetCharacters = 480
const maxReadCharacters = 8_000
const descriptionCharacters = 80
const bundleRoot = new URL("../../knowledge/docs/", import.meta.url)

/**
 * 随 Agent 构建打包的公开文档知识库：按 H2/H3 切分章节，按语言分别建立独立 BM25F 索引。
 * 文档内容是第一方参考资料，但仍只作为事实来源，不作为可执行指令。
 */
export class DocsKnowledge {
  readonly digest: string
  private readonly pages = new Map<string, DocsPage>()
  private readonly chunks = new Map<string, DocsChunk>()
  private readonly indices: Record<DocsLocale, BM25FIndex>
  private readonly navigation = new Map<string, NavigationItem[]>()

  constructor(
    files: Array<{ path: string, content: string }>,
    digest: string,
    private readonly baseUrl?: string,
    navigation: Array<{ path: string, content: string }> = [],
  ) {
    this.digest = digest
    for (const file of navigation) {
      const directory = file.path.replace(/\/?_(meta|nav)\.json$/u, "")
      const items = JSON.parse(file.content) as unknown
      if (!Array.isArray(items)) continue
      const entries = items.filter((item): item is NavigationItem => typeof item === "object" && item !== null)
      // 语言根目录的 _nav.json 是顶部导航：activeMatch 的首段即目录名，text 为标签。
      this.navigation.set(directory, file.path.endsWith("_nav.json")
        ? entries.map(item => ({ type: "dir", name: item.activeMatch?.split("/").filter(Boolean)[0] ?? "", label: item.text ?? "" }))
        : entries)
    }
    const chunksByLocale: Record<DocsLocale, DocsChunk[]> = { zh: [], en: [] }
    for (const file of files) {
      const page = parsePage(file.path, file.content)
      if (!page) continue
      this.pages.set(page.docId, page)
      for (const chunk of page.chunks) {
        this.chunks.set(chunk.key, chunk)
        chunksByLocale[chunk.locale].push(chunk)
      }
    }
    const buildIndex = (localeChunks: DocsChunk[]) => new BM25FIndex(localeChunks.map(chunk => ({
      operationId: chunk.key,
      fields: {
        title: { text: chunk.title, weight: 3 },
        heading: { text: chunk.heading, weight: 3 },
        path: { text: chunk.docId.replaceAll("/", " ").replaceAll("-", " "), weight: 1 },
        body: { text: chunk.body, weight: 1 },
      },
    })), new UnicodeLexicalTokenizer())
    this.indices = {
      zh: buildIndex(chunksByLocale.zh),
      en: buildIndex(chunksByLocale.en),
    }
  }

  search(query: string, locale: DocsLocale | undefined, limit: number): DocsSearchItem[] {
    if (limit <= 0) return []
    const candidateLimit = Math.max(limit * 4, 24)
    const ranked = locale
      ? this.indices[locale].search(query, candidateLimit)
      : [...this.indices.zh.search(query, candidateLimit), ...this.indices.en.search(query, candidateLimit)]
          .sort((left, right) => right.score - left.score || left.operationId.localeCompare(right.operationId))
    const items: DocsSearchItem[] = []
    const seenSections = new Set<string>()
    for (const match of ranked) {
      const chunk = this.chunks.get(match.operationId)
      if (!chunk) continue
      const sectionKey = `${chunk.docId}#${chunk.sectionId}`
      if (seenSections.has(sectionKey)) continue
      seenSections.add(sectionKey)
      items.push({
        docId: chunk.docId,
        sectionId: chunk.sectionId,
        title: chunk.title,
        section: chunk.heading,
        snippet: snippet(chunk.body),
        ...this.urlFields(chunk.docId, chunk.sectionId),
      })
      if (items.length >= limit) break
    }
    return items
  }

  /**
   * 按文档站目录树浏览：只返回目录名称与文档标题、简短描述，不含正文；顺序与标签遵循各目录 _meta.json。
   * path 为相对语言根目录的目录路径，空字符串表示根目录；depth 控制展开层数。
   */
  list(locale: DocsLocale, path: string, depth: number): DocsListResult | undefined {
    const directory = [locale, ...path.split("/").filter(Boolean)].join("/")
    if (directory !== locale && !this.hasDirectory(directory)) return undefined
    return { locale, path: directory.slice(locale.length + 1), entries: this.directoryEntries(directory, depth) }
  }

  read(docId: string, sectionId?: string): DocsReadResult | undefined {
    const page = this.pages.get(docId)
    if (!page) return undefined
    const sections = collectPageSections(page.chunks)
    const selected = sectionId
      ? page.chunks.filter(chunk => chunk.sectionId === sectionId || chunk.parentSectionId === sectionId)
      : page.chunks
    if (sectionId && !selected.length) return undefined
    const renderedParts: string[] = []
    let emittedParentSectionId: string | undefined
    for (const chunk of selected) {
      if (chunk.parentSectionId && chunk.parentHeading) {
        if (emittedParentSectionId !== chunk.parentSectionId) {
          const wantsParentHeading = !sectionId || sectionId === chunk.parentSectionId
          const hasExplicitParentChunk = selected.some(candidate => candidate.sectionId === chunk.parentSectionId)
          if (wantsParentHeading && !hasExplicitParentChunk) {
            renderedParts.push(`## ${chunk.parentHeading}`)
          }
          emittedParentSectionId = chunk.parentSectionId
        }
      }
      else if (chunk.sectionId) {
        emittedParentSectionId = chunk.sectionId
      }
      renderedParts.push(renderDocsChunk(chunk, page.title))
    }
    const full = renderedParts.join("\n\n").trim()
    const resolvedSectionHeading = sectionId
      ? (selected[0]!.sectionId === sectionId
          ? selected[0]!.heading
          : (selected[0]!.parentHeading ?? selected[0]!.heading))
      : undefined
    return {
      docId,
      title: page.title,
      ...(resolvedSectionHeading ? { section: resolvedSectionHeading } : {}),
      sections,
      content: full.slice(0, maxReadCharacters),
      truncated: full.length > maxReadCharacters,
      ...this.urlFields(docId, sectionId),
    }
  }

  private hasDirectory(directory: string): boolean {
    for (const docId of this.pages.keys()) if (docId.startsWith(`${directory}/`)) return true
    return false
  }

  private directoryEntries(directory: string, depth: number): DocsTreeEntry[] {
    const documents = new Map<string, DocsPage>()
    const directories = new Map<string, number>()
    for (const [docId, page] of this.pages) {
      if (!docId.startsWith(`${directory}/`)) continue
      const rest = docId.slice(directory.length + 1).split("/")
      if (rest.length === 1) documents.set(rest[0]!, page)
      else directories.set(rest[0]!, (directories.get(rest[0]!) ?? 0) + 1)
    }
    const meta = this.navigation.get(directory) ?? []
    const seenDirs = new Set<string>()
    const seenDocs = new Set<string>()
    const entries: DocsTreeEntry[] = []

    const pushDirectory = (name: string, metaLabel?: string) => {
      const documentCount = directories.get(name)
      if (documentCount === undefined || seenDirs.has(name)) return
      seenDirs.add(name)
      const childDirectory = `${directory}/${name}`
      const indexPage = this.pages.get(`${childDirectory}/index`)
      entries.push({
        type: "dir",
        path: childDirectory.split("/").slice(1).join("/"),
        label: metaLabel || indexPage?.title || documents.get(name)?.title || name,
        documentCount,
        ...(depth > 1 ? { children: this.directoryEntries(childDirectory, depth - 1) } : {}),
      })
    }

    const pushDocument = (name: string) => {
      const page = documents.get(name)
      if (!page || seenDocs.has(name)) return
      seenDocs.add(name)
      entries.push({ type: "doc", docId: page.docId, title: page.title, description: page.description })
    }

    for (const item of meta) {
      if (!item.name) continue
      if (item.type === "dir") {
        pushDirectory(item.name, item.label)
      }
      else if (item.type === "file") {
        pushDocument(item.name)
      }
      else {
        pushDirectory(item.name, item.label)
        pushDocument(item.name)
      }
    }

    const remainingNames = [...new Set([...directories.keys(), ...documents.keys()])].sort()
    for (const name of remainingNames) {
      const dirLabel = meta.find(item => item.name === name && item.type !== "file")?.label
      pushDirectory(name, dirLabel)
      pushDocument(name)
    }
    return entries
  }

  private urlFields(docId: string, sectionId?: string): { url?: string } {
    if (!this.baseUrl) return {}
    const [locale, ...rest] = docId.split("/")
    const path = rest.join("/").replace(/(^|\/)index$/u, "")
    const url = new URL(this.baseUrl)
    const basePath = url.pathname.replace(/^\/+|\/+$/gu, "")
    const segments = [
      ...(basePath ? [basePath] : []),
      ...(locale === "zh" ? [] : [locale]),
      ...(path ? [path.replace(/^\/+|\/+$/gu, "")] : []),
    ].filter(Boolean)
    url.pathname = segments.length ? `/${segments.join("/")}` : "/"
    url.search = ""
    url.hash = sectionId ? sectionId : ""
    return { url: url.toString() }
  }
}

let bundled: DocsKnowledge | undefined

/** 读取构建时打包的文档；缺少打包产物属于构建错误，直接失败而不是静默降级为空知识库。 */
export function loadBundledDocsKnowledge(baseUrl?: string): DocsKnowledge {
  const manifestUrl = new URL("manifest.json", bundleRoot)
  if (!existsSync(manifestUrl)) throw new Error("ai.docs_bundle_missing")
  const manifest = JSON.parse(readFileSync(manifestUrl, "utf8")) as DocsManifest
  const read = (path: string) => ({ path, content: readFileSync(new URL(path, bundleRoot), "utf8") })
  return new DocsKnowledge(manifest.files.map(read), manifest.digest, baseUrl, (manifest.navigation ?? []).map(read))
}

export function bundledDocsKnowledge(): DocsKnowledge {
  bundled ??= loadBundledDocsKnowledge()
  return bundled
}

function collectPageSections(chunks: DocsChunk[]): Array<{ sectionId: string, heading: string }> {
  const seen = new Set<string>()
  const sections: Array<{ sectionId: string, heading: string }> = []
  for (const chunk of chunks) {
    if (chunk.parentSectionId && chunk.parentHeading && !seen.has(chunk.parentSectionId)) {
      seen.add(chunk.parentSectionId)
      sections.push({ sectionId: chunk.parentSectionId, heading: chunk.parentHeading })
    }
    if (chunk.sectionId && !seen.has(chunk.sectionId)) {
      seen.add(chunk.sectionId)
      sections.push({ sectionId: chunk.sectionId, heading: chunk.heading })
    }
  }
  return sections
}

function renderDocsChunk(chunk: DocsChunk, pageTitle: string): string {
  if (chunk.continuation || !chunk.heading || chunk.level === 1 || chunk.heading === pageTitle) {
    return chunk.body
  }
  const hashes = chunk.level === 3 ? "###" : "##"
  return `${hashes} ${chunk.heading}\n\n${chunk.body}`
}

function splitLongTextByParagraphs(text: string, maxChars: number): string[] {
  const trimmed = text.trim()
  if (!trimmed) return []
  if (trimmed.length <= maxChars) return [trimmed]
  const paragraphs = trimmed.split(/\n\s*\n/u)
  const pieces: string[] = []
  let current = ""
  for (const rawParagraph of paragraphs) {
    const paragraph = rawParagraph.trim()
    if (!paragraph) continue
    if (!current) {
      current = paragraph
      continue
    }
    if (current.length + 2 + paragraph.length <= maxChars) {
      current = `${current}\n\n${paragraph}`
      continue
    }
    pieces.push(current)
    current = paragraph
  }
  if (current) pieces.push(current)
  return pieces
}

function parsePage(path: string, content: string): DocsPage | undefined {
  const match = /^(zh|en)\/(.+)\.mdx?$/u.exec(path)
  if (!match) return undefined
  const locale = match[1] as DocsLocale
  const docId = `${locale}/${match[2]}`
  const frontmatter = frontmatterFields(content)
  const lines = stripFrontmatter(content).split(/\r?\n/u)
  let title = frontmatter.title ?? ""
  const sections: Array<{ heading: string, level: 1 | 2 | 3, lines: string[] }> = [{ heading: "", level: 1, lines: [] }]
  let inFence = false
  for (const line of lines) {
    if (/^\s*(```|~~~)/u.test(line)) inFence = !inFence
    const heading = inFence ? undefined : /^(#{1,3})\s+(.+?)\s*#*\s*$/u.exec(line)
    if (heading?.[1] === "#" && !title) {
      title = heading[2]!
      continue
    }
    if (heading && heading[1]!.length >= 2) {
      const level = heading[1]!.length === 3 ? 3 : 2
      sections.push({ heading: heading[2]!, level, lines: [] })
      continue
    }
    sections[sections.length - 1]!.lines.push(line)
  }
  title ||= match[2]!.split("/").at(-1)!

  // H3 默认并入所属 H2；只有单个 H2 章节过长时才拆分为子片段，且完整保留 level 与 parentSectionId。
  const usedSlugs = new Map<string, number>()
  const uniqueSlug = (heading: string): string => {
    const base = slug(heading)
    if (!base) return ""
    const count = usedSlugs.get(base) ?? 0
    usedSlugs.set(base, count + 1)
    return count === 0 ? base : `${base}-${count}`
  }

  const grouped: Array<{
    heading: string
    sectionId: string
    level: 1 | 2
    parts: Array<{ heading: string, sectionId: string, level: 1 | 2 | 3, body: string }>
  }> = []
  for (const section of sections) {
    const body = section.lines.join("\n").trim()
    const sectionId = section.level >= 2 && section.heading ? uniqueSlug(section.heading) : ""
    if (section.level <= 2 || !grouped.length) {
      const groupLevel: 1 | 2 = section.level === 1 ? 1 : 2
      grouped.push({
        heading: section.heading,
        sectionId,
        level: groupLevel,
        parts: [{ heading: section.heading, sectionId, level: section.level, body }],
      })
    }
    else {
      grouped[grouped.length - 1]!.parts.push({ heading: section.heading, sectionId, level: 3, body })
    }
  }

  const chunks: DocsChunk[] = []
  const appendPartChunks = (input: {
    heading: string
    level: 1 | 2 | 3
    sectionId: string
    parentSectionId?: string
    parentHeading?: string
    body: string
  }) => {
    const segments = splitLongTextByParagraphs(input.body, maxChunkCharacters)
    segments.forEach((segment, segmentIndex) => {
      chunks.push({
        key: `${docId}#${input.sectionId}#${chunks.length}`,
        docId,
        locale,
        sectionId: input.sectionId,
        ...(input.parentSectionId ? { parentSectionId: input.parentSectionId } : {}),
        ...(input.parentHeading ? { parentHeading: input.parentHeading } : {}),
        level: input.level,
        ...(segmentIndex > 0 ? { continuation: true } : {}),
        title,
        heading: input.heading || title,
        body: segment,
      })
    })
  }

  for (const group of grouped) {
    const combinedBody = group.parts
      .map((part, index) => {
        if (index === 0) return part.body
        return part.body ? `### ${part.heading}\n\n${part.body}` : `### ${part.heading}`
      })
      .filter(Boolean)
      .join("\n\n")
      .trim()
    if (!combinedBody && !group.heading) continue
    const groupSectionId = group.sectionId

    if (combinedBody.length > maxChunkCharacters && group.parts.length > 1) {
      for (const [index, part] of group.parts.entries()) {
        if (!part.body) continue
        if (index === 0) {
          appendPartChunks({
            heading: group.heading,
            level: group.level,
            sectionId: groupSectionId,
            body: part.body,
          })
        }
        else {
          appendPartChunks({
            heading: part.heading,
            level: 3,
            sectionId: part.sectionId,
            ...(groupSectionId ? { parentSectionId: groupSectionId, parentHeading: group.heading } : {}),
            body: part.body,
          })
        }
      }
      continue
    }

    if (!combinedBody) continue
    appendPartChunks({
      heading: group.heading,
      level: group.level,
      sectionId: groupSectionId,
      body: combinedBody,
    })
  }
  return { docId, title, description: frontmatter.description ? truncateDescription(frontmatter.description) : describe(chunks[0]?.body ?? ""), chunks }
}

/** 文档开头第一句话作为目录树中的简短描述，去掉 Markdown 标记。 */
function describe(body: string): string {
  const firstParagraph = body.split(/\n\s*\n/u).find(paragraph => paragraph.trim() && !/^\s*([-*>|]|```|<)/u.test(paragraph)) ?? ""
  const plain = firstParagraph.replace(/[*_`]/gu, "").replace(/\[([^\]]+)\]\([^)]*\)/gu, "$1").replace(/\s+/gu, " ").trim()
  // 中文句末标点直接断句；英文句点需后跟空白或结尾，避免把 1.2 之类版本号切开。
  const sentence = /^.+?(?:[。！？]|[.!?](?=\s|$))/u.exec(plain)?.[0] ?? plain
  return truncateDescription(sentence)
}

function truncateDescription(value: string): string {
  return value.length > descriptionCharacters ? `${value.slice(0, descriptionCharacters)}…` : value
}

/** 只解析目录树需要的标量字段：title、description；首页没有 title 时用 hero.name。 */
function frontmatterFields(content: string): { title?: string, description?: string } {
  const block = /^---\r?\n([\s\S]*?)\r?\n---/u.exec(content)?.[1]
  if (!block) return {}
  const scalar = (pattern: RegExp) => pattern.exec(block)?.[1]?.trim().replace(/^["']|["']$/gu, "") || undefined
  const title = scalar(/^title:\s*(.+)$/mu) ?? scalar(/^hero:\s*\n\s+name:\s*(.+)$/mu)
  const description = scalar(/^description:\s*(.+)$/mu)
  return { ...(title ? { title } : {}), ...(description ? { description } : {}) }
}

function stripFrontmatter(content: string): string {
  return content.startsWith("---") ? content.replace(/^---\r?\n[\s\S]*?\r?\n---\r?\n?/u, "") : content
}

/** 与文档站标题锚点一致的简单 slug：小写、去除标点、空白转连字符，保留中文。 */
function slug(heading: string): string {
  return heading
    .replace(/`/gu, "")
    .toLocaleLowerCase("en-US")
    .replace(/[^\p{Letter}\p{Number}\s-]/gu, "")
    .trim()
    .replace(/\s+/gu, "-")
}

function snippet(body: string): string {
  const plain = body.replace(/\s+/gu, " ").trim()
  return plain.length > snippetCharacters ? `${plain.slice(0, snippetCharacters)}…` : plain
}
