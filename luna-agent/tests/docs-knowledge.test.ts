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
})
