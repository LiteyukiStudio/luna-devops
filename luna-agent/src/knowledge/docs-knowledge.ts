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
 * 随 Agent 构建打包的公开文档知识库：按 H2/H3 切分章节，用与工具目录相同的中英文分词和 BM25F 检索。
 * 文档内容是第一方参考资料，但仍只作为事实来源，不作为可执行指令。
 */
export class DocsKnowledge {
  readonly digest: string
  private readonly pages = new Map<string, DocsPage>()
  private readonly chunks = new Map<string, DocsChunk>()
  private readonly index: BM25FIndex
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
    for (const file of files) {
      const page = parsePage(file.path, file.content)
      if (!page) continue
      this.pages.set(page.docId, page)
      for (const chunk of page.chunks) this.chunks.set(chunk.key, chunk)
    }
    this.index = new BM25FIndex([...this.chunks.values()].map(chunk => ({
      operationId: chunk.key,
      fields: {
        title: { text: chunk.title, weight: 3 },
        heading: { text: chunk.heading, weight: 3 },
        path: { text: chunk.docId.replaceAll("/", " ").replaceAll("-", " "), weight: 1 },
        body: { text: chunk.body, weight: 1 },
      },
    })), new UnicodeLexicalTokenizer())
  }

  search(query: string, locale: DocsLocale | undefined, limit: number): DocsSearchItem[] {
    const ranked = this.index.search(query, this.chunks.size)
    const items: DocsSearchItem[] = []
    for (const match of ranked) {
      const chunk = this.chunks.get(match.operationId)
      if (!chunk || (locale && chunk.locale !== locale)) continue
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
    const sections = page.chunks.filter(chunk => chunk.sectionId).map(chunk => ({ sectionId: chunk.sectionId, heading: chunk.heading }))
    const selected = sectionId ? page.chunks.filter(chunk => chunk.sectionId === sectionId) : page.chunks
    if (sectionId && !selected.length) return undefined
    const full = selected.map(chunk => chunk.heading && chunk.heading !== page.title ? `## ${chunk.heading}\n\n${chunk.body}` : chunk.body).join("\n\n").trim()
    return {
      docId,
      title: page.title,
      ...(sectionId ? { section: selected[0]!.heading } : {}),
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
    const ordered = [
      ...meta.map(item => item.name).filter((name): name is string => Boolean(name)),
      ...[...directories.keys(), ...documents.keys()].sort(),
    ]
    const seen = new Set<string>()
    const entries: DocsTreeEntry[] = []
    for (const name of ordered) {
      if (seen.has(name)) continue
      seen.add(name)
      const label = meta.find(item => item.name === name)?.label
      const childDirectory = `${directory}/${name}`
      const documentCount = directories.get(name)
      if (documentCount !== undefined) {
        entries.push({
          type: "dir",
          path: childDirectory.split("/").slice(1).join("/"),
          label: label || documents.get(name)?.title || name,
          documentCount,
          ...(depth > 1 ? { children: this.directoryEntries(childDirectory, depth - 1) } : {}),
        })
        continue
      }
      const page = documents.get(name)
      if (page) entries.push({ type: "doc", docId: page.docId, title: page.title, description: page.description })
    }
    return entries
  }

  private urlFields(docId: string, sectionId?: string): { url?: string } {
    if (!this.baseUrl) return {}
    const [locale, ...rest] = docId.split("/")
    const path = rest.join("/").replace(/(^|\/)index$/u, "")
    const prefix = locale === "zh" ? "" : `/${locale}`
    const url = new URL(`${prefix}/${path}`.replace(/\/+$/u, "") || "/", this.baseUrl)
    if (sectionId) url.hash = sectionId
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

function parsePage(path: string, content: string): DocsPage | undefined {
  const match = /^(zh|en)\/(.+)\.mdx?$/u.exec(path)
  if (!match) return undefined
  const locale = match[1] as DocsLocale
  const docId = `${locale}/${match[2]}`
  const frontmatter = frontmatterFields(content)
  const lines = stripFrontmatter(content).split(/\r?\n/u)
  let title = frontmatter.title ?? ""
  const sections: Array<{ heading: string, level: number, lines: string[] }> = [{ heading: "", level: 1, lines: [] }]
  let inFence = false
  for (const line of lines) {
    if (/^\s*(```|~~~)/u.test(line)) inFence = !inFence
    const heading = inFence ? undefined : /^(#{1,3})\s+(.+?)\s*#*\s*$/u.exec(line)
    if (heading?.[1] === "#" && !title) {
      title = heading[2]!
      continue
    }
    if (heading && heading[1]!.length >= 2) {
      sections.push({ heading: heading[2]!, level: heading[1]!.length, lines: [] })
      continue
    }
    sections[sections.length - 1]!.lines.push(line)
  }
  title ||= match[2]!.split("/").at(-1)!

  // H3 并入所属 H2；只有单个 H2 章节过长时才按 H3 继续拆分，保持检索粒度与上下文完整性的平衡。
  const grouped: Array<{ heading: string, parts: Array<{ heading: string, text: string }> }> = []
  for (const section of sections) {
    const text = section.lines.join("\n").trim()
    if (section.level <= 2 || !grouped.length) grouped.push({ heading: section.heading, parts: [{ heading: section.heading, text }] })
    else grouped[grouped.length - 1]!.parts.push({ heading: section.heading, text: `### ${section.heading}\n\n${text}` })
  }

  const chunks: DocsChunk[] = []
  for (const group of grouped) {
    const body = group.parts.map(part => part.text).join("\n\n").trim()
    if (!body && !group.heading) continue
    const pieces = body.length > maxChunkCharacters && group.parts.length > 1
      ? group.parts.map((part, index) => ({ heading: index === 0 ? group.heading : part.heading, body: part.text.trim() }))
      : [{ heading: group.heading, body }]
    for (const piece of pieces) {
      if (!piece.body) continue
      const sectionId = piece.heading ? slug(piece.heading) : ""
      chunks.push({ key: `${docId}#${sectionId}#${chunks.length}`, docId, locale, sectionId, title, heading: piece.heading || title, body: piece.body })
    }
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
