import { describe, expect, it } from "vitest"
import { DocsKnowledge } from "../src/knowledge/docs-knowledge.js"

const files = [
  {
    path: "zh/use/workflow/deployment.md",
    content: [
      "# 创建部署配置",
      "",
      "部署配置描述应用从哪里来、运行在哪里、如何运行。第二句不进入描述。",
      "",
      "## 必填内容",
      "",
      "- 阶段：`dev`、`test`、`staging` 或 `prod`，创建后不可修改。",
      "",
      "```yaml",
      "# 这不是标题",
      "stage: prod",
      "```",
      "",
      "## 健康检查",
      "",
      "### HTTP 探针",
      "",
      "探针路径以 / 开头。",
    ].join("\n"),
  },
  {
    path: "zh/use/workflow/routes.md",
    content: "# 访问入口\n\n## 自定义域名\n\n域名需要先完成 DNS 解析。",
  },
  {
    path: "zh/reference/image-template.md",
    content: "---\ntitle: 镜像模板变量\ndescription: 镜像仓库路径与标签模板支持的变量。\n---\n\n## 变量列表\n\n`{registryNamespace}` 镜像站命名空间。",
  },
  {
    path: "en/use/workflow/deployment.md",
    content: "# Create a deployment config\n\n## Required fields\n\nStage is one of dev, test, staging or prod.",
  },
  {
    path: "zh/index.md",
    content: "---\ndescription: 文档首页。\npageType: home\nhero:\n  name: Luna DevOps\n---\n",
  },
]
const navigation = [
  { path: "zh/_nav.json", content: JSON.stringify([{ text: "使用", activeMatch: "/use/" }, { text: "参考", activeMatch: "/reference/" }]) },
  { path: "zh/use/workflow/_meta.json", content: JSON.stringify([{ type: "file", name: "routes", label: "访问入口" }, { type: "file", name: "deployment", label: "部署配置" }]) },
  { path: "zh/use/_meta.json", content: JSON.stringify([{ type: "dir", name: "workflow", label: "日常交付" }]) },
]

function knowledge(baseUrl?: string) {
  return new DocsKnowledge(files, "digest", baseUrl, navigation)
}

describe("bundled docs knowledge", () => {
  it("finds sections by heading and body text and filters by locale", () => {
    const items = knowledge().search("部署配置 阶段 prod", "zh", 3)
    expect(items[0]).toMatchObject({ docId: "zh/use/workflow/deployment", section: "必填内容", sectionId: "必填内容" })
    expect(items.every(item => item.docId.startsWith("zh/"))).toBe(true)
    expect(knowledge().search("required stage", "en", 3)[0]?.docId).toBe("en/use/workflow/deployment")
    expect(knowledge().search("完全无关的词语组合", "zh", 3)).toEqual([])
  })

  it("keeps code fence lines out of headings and folds H3 into its H2 section", () => {
    const document = knowledge().read("zh/use/workflow/deployment")
    expect(document?.sections.map(section => section.heading)).toEqual(["必填内容", "健康检查"])
    expect(document?.content).toContain("# 这不是标题")
    expect(knowledge().read("zh/use/workflow/deployment", "健康检查")?.content).toContain("### HTTP 探针")
  })

  it("reads frontmatter titles and rejects unknown documents or sections", () => {
    expect(knowledge().read("zh/reference/image-template")?.title).toBe("镜像模板变量")
    expect(knowledge().read("zh/missing")).toBeUndefined()
    expect(knowledge().read("zh/use/workflow/deployment", "不存在")).toBeUndefined()
  })

  it("lists the docs tree with navigation labels, order and one-line descriptions only", () => {
    const root = knowledge().list("zh", "", 2)
    expect(root?.entries.map(entry => entry.type === "dir" ? entry.label : entry.title)).toEqual(["使用", "参考", "Luna DevOps"])
    const workflow = knowledge().list("zh", "use/workflow", 1)
    expect(workflow?.entries).toEqual([
      { type: "doc", docId: "zh/use/workflow/routes", title: "访问入口", description: "域名需要先完成 DNS 解析。" },
      { type: "doc", docId: "zh/use/workflow/deployment", title: "创建部署配置", description: "部署配置描述应用从哪里来、运行在哪里、如何运行。" },
    ])
    expect(JSON.stringify(workflow)).not.toContain("staging")
    expect(knowledge().list("zh", "reference", 1)?.entries[0]).toMatchObject({ title: "镜像模板变量", description: "镜像仓库路径与标签模板支持的变量。" })
    expect(knowledge().list("zh", "nope", 1)).toBeUndefined()
  })

  it("links to the docs site only when a base URL is configured", () => {
    expect(knowledge().search("自定义域名", "zh", 1)[0]?.url).toBeUndefined()
    expect(knowledge("https://docs.example.com").search("自定义域名", "zh", 1)[0]?.url).toBe(`https://docs.example.com/use/workflow/routes#${encodeURIComponent("自定义域名")}`)
    expect(knowledge("https://docs.example.com").read("en/use/workflow/deployment")?.url).toBe("https://docs.example.com/en/use/workflow/deployment")
  })

  it("isolates zh and en BM25 indices, preserves sibling dir/doc + index entries, and concatenates split long H2 with accurate ### levels", () => {
    const longPartA = `排障步骤 A。${"检查镜像仓库凭据与网络连通性。".repeat(95)}`
    const longPartB = `排障步骤 B。${"检查容器端口与就绪探针超时配置。".repeat(95)}`
    const customDocs = new DocsKnowledge([
      {
        path: "zh/guide/index.md",
        content: "---\ntitle: 快速上手总览\ndescription: 新手入门导览。\n---\n\n欢迎阅读快速上手。",
      },
      {
        path: "zh/guide/troubleshooting.md",
        content: [
          "# 排障指南",
          "",
          "## 发布失败排查",
          "",
          "这是导语说明。",
          "",
          "### 镜像拉取异常",
          "",
          longPartA,
          "",
          "### 探针健康检查失败",
          "",
          longPartB,
        ].join("\n"),
      },
      {
        path: "zh/guide.md",
        content: "# 指南速查单页\n\n同级独立说明页。",
      },
      {
        path: "en/guide/index.md",
        content: "# Quickstart Overview\n\n## Troubleshooting\n\nUniqueSharedToken appears in English many times UniqueSharedToken UniqueSharedToken.",
      },
      {
        path: "zh/guide/token.md",
        content: "# 令牌说明\n\n## 唯一标识\n\nUniqueSharedToken 在中文页出现一次。",
      },
    ], "digest-custom")

    // 1) Separate BM25 indices per locale: zh search returns only zh docs, en search returns only en docs
    const zhHits = customDocs.search("UniqueSharedToken", "zh", 5)
    expect(zhHits).toHaveLength(1)
    expect(zhHits[0]?.docId).toBe("zh/guide/token")
    const enHits = customDocs.search("UniqueSharedToken", "en", 5)
    expect(enHits).toHaveLength(1)
    expect(enHits[0]?.docId).toBe("en/guide/index")

    // 2) Listing root preserves BOTH directory zh/guide (labeled from zh/guide/index title) AND sibling doc zh/guide.md
    const zhRoot = customDocs.list("zh", "", 2)
    expect(zhRoot?.entries).toEqual([
      expect.objectContaining({
        type: "dir",
        path: "guide",
        label: "快速上手总览",
      }),
      expect.objectContaining({
        type: "doc",
        docId: "zh/guide",
        title: "指南速查单页",
      }),
    ])
    const guideDir = customDocs.list("zh", "guide", 1)
    expect(guideDir?.entries.map(entry => entry.type === "doc" ? entry.docId : entry.path)).toContain("zh/guide/index")

    // 3) Reading a split H3 renders ### (never ##), and reading the parent H2 concatenates intro + all H3 sub-chunks
    const h3Read = customDocs.read("zh/guide/troubleshooting", "镜像拉取异常")
    expect(h3Read?.content.startsWith("### 镜像拉取异常")).toBe(true)
    expect(h3Read?.content).not.toMatch(/^## 镜像拉取异常/m)

    const h2Read = customDocs.read("zh/guide/troubleshooting", "发布失败排查")
    expect(h2Read?.section).toBe("发布失败排查")
    expect(h2Read?.content).toContain("## 发布失败排查")
    expect(h2Read?.content).toContain("这是导语说明。")
    expect(h2Read?.content).toContain("### 镜像拉取异常")
    expect(h2Read?.content).toContain("### 探针健康检查失败")
    expect(h2Read?.content).toContain("排障步骤 B。")
  })

  it("disambiguates duplicate section headings, emits intro-less H2 headings on full read, and preserves baseUrl pathname prefixes", () => {
    const longBodyA = `${"构建参数说明。".repeat(220)}\n\n${"构建约束说明。".repeat(220)}`
    const longBodyB = `${"发布参数说明。".repeat(220)}\n\n${"发布约束说明。".repeat(220)}`
    const docs = new DocsKnowledge([
      {
        path: "zh/use/pipeline.md",
        content: `# 流水线参考\n\n## 构建阶段\n\n### 参数\n\n${longBodyA}\n\n## 发布阶段\n\n### 参数\n\n${longBodyB}\n`,
      },
      {
        path: "en/use/pipeline.md",
        content: "# Pipeline Reference\n\n## Overview\n\nPipeline steps.\n",
      },
    ], "sha256:dup-slug", "https://example.com/docs/")

    const zhRead = docs.read("zh/use/pipeline")
    expect(zhRead?.content).toContain("## 构建阶段")
    expect(zhRead?.content).toContain("## 发布阶段")
    const sectionIds = zhRead?.sections.map(section => section.sectionId) ?? []
    expect(sectionIds).toContain("参数")
    expect(sectionIds).toContain("参数-1")
    expect(new Set(sectionIds).size).toBe(sectionIds.length)

    expect(docs.read("zh/use/pipeline", "参数")?.content).toContain("构建参数说明。")
    expect(docs.read("zh/use/pipeline", "参数")?.content).not.toContain("发布参数说明。")
    expect(docs.read("zh/use/pipeline", "参数-1")?.content).toContain("发布参数说明。")
    expect(docs.read("zh/use/pipeline", "参数-1")?.url).toBe("https://example.com/docs/use/pipeline#%E5%8F%82%E6%95%B0-1")
    expect(docs.read("en/use/pipeline")?.url).toBe("https://example.com/docs/en/use/pipeline")

    // Search deduplicates continuation chunks of the same (docId, sectionId) so both distinct sections surface cleanly
    const searchHits = docs.search("参数说明 约束说明", "zh", 5)
    expect(searchHits).toHaveLength(2)
    expect(new Set(searchHits.map(hit => hit.sectionId))).toEqual(new Set(["参数", "参数-1"]))
  })
})
