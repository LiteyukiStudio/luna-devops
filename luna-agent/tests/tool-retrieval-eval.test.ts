import { describe, expect, it } from "vitest"
import { ToolCatalog } from "../src/tools/catalog.js"

const definitions = [
  ["listProjects", "projects", "列出项目空间", "List project workspaces"],
  ["createProject", "projects", "创建项目空间", "Create a project workspace"],
  ["listApplications", "applications", "列出项目空间内的应用", "List applications"],
  ["createApplication", "applications", "创建承载业务服务的应用", "Create an application"],
  ["listAppTemplates", "applications", "搜索和比较应用市场模板", "List app marketplace templates"],
  ["getAppTemplate", "applications", "读取单个模板的安装参数", "Get app template values"],
  ["installAppTemplate", "applications", "安装已选定的应用市场模板", "Install app template"],
  ["listRegistryCredentials", "registries", "检查镜像站推送凭据", "List registry credentials"],
  ["triggerBuildRun", "builds", "从代码仓库启动源码构建", "Trigger source build"],
  ["retryBuildRun", "builds", "重试失败的源码构建", "Retry source build"],
  ["createRelease", "releases", "使用镜像创建发布", "Create an image release"],
  ["createGatewayRoute", "gateway", "创建域名和公网访问入口", "Create a public gateway route"],
  ["listGatewayRoutes", "gateway", "查看已有域名和访问入口", "List gateway routes"],
  ["getReleaseRuntimeLogs", "runtime", "读取发布的容器运行日志", "Get release runtime logs"],
  ["listRuntimeEvents", "runtime", "查看 Pod 调度、拉取镜像和卷挂载事件", "List Kubernetes runtime events"],
  ["webSearch", "platform", "搜索互联网和官方资料", "Search the public web"],
  ["fetchWebPage", "platform", "读取明确网址、README 和部署文档", "Fetch a web page"],
  ["updateDeploymentTargetRuntimeSecrets", "deployment", "安全更新部署目标的运行时密钥", "Update deployment runtime secrets"],
  ["updateProjectRuntimeConfigSetRuntimeSecrets", "deployment", "安全更新运行时配置集的密钥变量", "Update runtime config set secrets"],
  ["createNotificationChannel", "notifications", "", "Create notification channel"],
  ["listNotificationChannels", "notifications", "", "List notification channels"],
] as const

const catalog = ToolCatalog.load(definitions.map(([operationId, category, description, hint]) => ({
  operationId,
  method: operationId.startsWith("list") || operationId.startsWith("get") || operationId.startsWith("fetch") ? "GET" : "POST",
  path: `/api/v1/eval/${operationId}`,
  category,
  description,
  searchHints: [hint],
  risk: "read",
  requiredScopes: ["project:read"],
  approval: "never",
  idempotent: true,
  timeoutMs: 30_000,
  inputSchema: { type: "object", properties: {}, required: [], additionalProperties: false },
})))

describe("tool retrieval evaluation", () => {
  it.each([
    ["帮我创建一个新的项目空间", "createProject"],
    ["看看这个项目空间有哪些应用", "listApplications"],
    ["从应用市场找 PostgreSQL 模板", "listAppTemplates"],
    ["读取选中模板需要填写的参数", "getAppTemplate"],
    ["按刚才确认的参数安装模板", "installAppTemplate"],
    ["源码构建前检查目标镜像站有没有推送凭据", "listRegistryCredentials"],
    ["重试项目空间 A 的构建前，检查这个项目能否使用镜像站推送凭据", "listRegistryCredentials"],
    ["这个仓库需要从源码开始构建", "triggerBuildRun"],
    ["原来的构建失败了，修好后重新构建", "retryBuildRun"],
    ["用官方镜像创建一个发布", "createRelease"],
    ["给服务配置一个公网域名", "createGatewayRoute"],
    ["现在已经有哪些访问地址", "listGatewayRoutes"],
    ["查看这个发布的容器日志", "getReleaseRuntimeLogs"],
    ["为什么 Pod 一直调度失败", "listRuntimeEvents"],
    ["搜索项目的官方部署说明", "webSearch"],
    ["读取这个 GitHub README 链接", "fetchWebPage"],
    ["给部署目标安全填写运行时密码", "updateDeploymentTargetRuntimeSecrets"],
    ["给运行时配置集绑定密钥变量", "updateProjectRuntimeConfigSetRuntimeSecrets"],
    ["创建一个新的通知渠道", "createNotificationChannel"],
    ["查询现有通知渠道", "listNotificationChannels"],
  ])("puts the expected operation in the first eight results: %s", (query, expected) => {
    const result = catalog.search(query, {}, 8)
    expect(result.loadedOperationIds, JSON.stringify(result.matches)).toContain(expected)
  })

  it("keeps discovery results bounded and returns actionable Chinese descriptions", () => {
    const result = catalog.search("部署一个公网可访问的应用", {}, 5)
    expect(result.matches).toHaveLength(5)
    expect(result.totalMatches).toBeGreaterThanOrEqual(result.matches.length)
    expect(result.matches.every(match => match.description.includes("适用：") || match.description.includes("只有用户需要"))).toBe(true)
  })
})
