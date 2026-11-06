import { describe, expect, it } from "vitest"
import { loadBundledDocsKnowledge } from "../src/knowledge/docs-knowledge.js"

// 随包公开文档的检索回归集：常见用户问法必须在前 3 条命中对应文档，防止文档改版或分词调整后检索退化。
const zhQuestions: Array<[string, string]> = [
  ["部署配置的阶段可以填哪些", "zh/use/workflow/deployment"],
  ["怎么回滚到上一个版本", "zh/use/workflow/build-and-release"],
  ["数据卷导出一致性怎么选", "zh/use/workflow/runtime-data-persistence"],
  ["访问入口怎么配置域名和 TLS", "zh/use/workflow/routes"],
  ["运行环境变量和密钥怎么配置", "zh/use/workflow/configuration-and-secrets"],
  ["部署配置怎么导出成 JSON 再导入", "zh/use/workflow/deployment-config-transfer"],
  ["账单里的费用是怎么算的", "zh/use/billing"],
  ["应用市场安装模板", "zh/use/app-marketplace"],
  ["发布失败怎么排查", "zh/use/diagnostics"],
  ["怎么配置通知 Webhook", "zh/use/notifications"],
  ["Luna CLI 怎么安装", "zh/use/cli/installation"],
  ["首次登录管理员账号", "zh/start/first-login"],
  ["目标镜像模板支持哪些变量", "zh/reference/image-templates"],
  ["镜像 tag 模板怎么写提交哈希", "zh/reference/image-templates"],
  ["构建参数 build args 的格式", "zh/reference/build-variables"],
  ["构建变量和 build args 同名谁生效", "zh/reference/build-variables"],
  ["环境变量怎么引用另一个变量", "zh/reference/runtime-variables"],
  ["应用市场模板的密码是怎么生成的", "zh/reference/app-template-parameters"],
  ["CPU 和内存配额的格式", "zh/reference/field-formats"],
  ["触发规则的通配符怎么匹配", "zh/reference/field-formats"],
]

const enQuestions: Array<[string, string]> = [
  ["Which variables can the image repository template use", "en/reference/image-templates"],
  ["build args format", "en/reference/build-variables"],
  ["How do I roll back a release", "en/use/workflow/build-and-release"],
]

describe("bundled docs retrieval regression", () => {
  const knowledge = loadBundledDocsKnowledge()

  it.each(zhQuestions)("“%s” 命中 %s", (question, docId) => {
    expect(knowledge.search(question, "zh", 3).map(item => item.docId)).toContain(docId)
  })

  it.each(enQuestions)("\"%s\" hits %s", (question, docId) => {
    expect(knowledge.search(question, "en", 3).map(item => item.docId)).toContain(docId)
  })

  it("reaches documents that keyword search misses by browsing the tree", () => {
    // 文档写的是“访问入口 / 域名”，用户说“自定义域名”时关键词检索可能落空，目录树仍能逐级定位。
    const use = knowledge.list("zh", "", 1)?.entries.find(entry => entry.type === "dir" && entry.path === "use")
    expect(use).toMatchObject({ label: "使用" })
    const workflow = knowledge.list("zh", "use", 1)?.entries.find(entry => entry.type === "dir" && entry.path === "use/workflow")
    expect(workflow).toMatchObject({ label: "日常交付" })
    const routes = knowledge.list("zh", "use/workflow", 1)?.entries.find(entry => entry.type === "doc" && entry.docId === "zh/use/workflow/routes")
    expect(routes).toMatchObject({ title: "配置访问入口" })
    expect(knowledge.read("zh/use/workflow/routes")?.content).toContain("域名")
  })

  it("lists every bundled document from the root tree", () => {
    const docIds: string[] = []
    const walk = (path: string) => {
      for (const entry of knowledge.list("zh", path, 1)?.entries ?? []) {
        if (entry.type === "doc") docIds.push(entry.docId)
        else walk(entry.path)
      }
    }
    walk("")
    expect(docIds.length).toBeGreaterThan(20)
    expect(new Set(docIds).size).toBe(docIds.length)
    expect(docIds).toContain("zh/use/workflow/deployment")
  })
})
