import { createHmac } from "node:crypto"
import { describe, expect, it } from "vitest"
import { BffHmacRequestVerifier, DevelopmentRequestVerifier } from "../src/auth.js"
import { loadConfig } from "../src/config.js"
import { HeadlessInferenceRunner } from "../src/headless-inference.js"
import type { ModelCapabilities, ModelEvent, ModelProvider, ModelRequest, ModelResponse } from "../src/provider/provider.js"
import { UsageRecordingModelProvider } from "../src/provider/usage-recorder.js"
import { ToolCatalog } from "../src/tools/catalog.js"
import { ToolCatalogRegistry } from "../src/tools/catalog-registry.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { testRemoteConfigSource, testRemoteProviderConfig } from "./support/remote-config.js"
import { buildTestServer } from "./support/server.js"
import { TestRepository } from "./support/test-repository.js"
import { testToolOperation } from "./support/tool-catalog.js"

class SequenceProvider implements ModelProvider {
  readonly requests: ModelRequest[] = []
  private index = 0

  constructor(private readonly steps: Array<(request: ModelRequest) => ModelResponse>) {}

  capabilities(): ModelCapabilities {
    return { streaming: true, toolCalling: true, structuredOutput: true }
  }

  async health() {
    return { ok: true }
  }

  async complete(request: ModelRequest): Promise<ModelResponse> {
    this.requests.push(request)
    const fn = this.steps[this.index] ?? this.steps.at(-1)!
    this.index += 1
    return fn(request)
  }

  async *stream(): AsyncIterable<ModelEvent> {
    yield {
      type: "completed",
      usage: { status: "reported", value: { inputTokens: 10, outputTokens: 5, totalTokens: 15 } },
    }
  }
}

describe("POST /internal/v1/inference (HeadlessInferenceRunner)", () => {
  it("runs single-shot repo_config_infer, records personal wallet usage, and does not pollute conversations", async () => {
    const repository = new TestRepository()
    repository.setWalletBalance("usr_dev", 100)
    const rawProvider = new SequenceProvider([
      () => ({
        text: "",
        toolCalls: [
          {
            id: "call_1",
            operationId: "submit_structured_output",
            arguments: {
              summary: "检测到 Node.js 服务 Dockerfile，监听 3000 端口。",
              confidence: "high",
              buildDefinitionMode: "repository_dockerfile",
              dockerfilePath: "Dockerfile",
              buildContext: ".",
              servicePorts: [{ name: "http", port: 3000, appProtocol: "http" }],
              environmentVariables: [{ key: "DATABASE_URL", secret: true, placeholder: "postgres://..." }],
              cpuRequest: "500m",
              memoryRequest: "512Mi",
            },
          },
        ],
        usage: { status: "reported", value: { inputTokens: 420, outputTokens: 110, totalTokens: 530 } },
      }),
    ])
    const provider = new UsageRecordingModelProvider(rawProvider, repository)
    const remote = testRemoteConfigSource(testRemoteProviderConfig())
    const headlessInference = new HeadlessInferenceRunner({
      repository,
      provider,
      remoteConfig: remote.source,
    })
    const app = buildTestServer({
      config: loadConfig({ NODE_ENV: "test" }),
      repository,
      requestVerifier: new DevelopmentRequestVerifier(),
      remoteConfig: remote.source,
      headlessInference,
    })

    const res = await app.inject({
      method: "POST",
      url: "/internal/v1/inference",
      headers: { "x-luna-dev-user": "usr_dev" },
      payload: {
        actorUserId: "usr_dev",
        projectId: "prj_1",
        taskType: "repo_config_infer",
        maxToolTier: "none",
        contextPayload: {
          owner: "acme",
          repo: "web-api",
          dockerfiles: ["Dockerfile"],
          exposedPorts: { Dockerfile: [3000] },
        },
      },
    })

    expect(res.statusCode).toBe(200)
    const body = res.json<{
      runId: string
      taskType: string
      output: Record<string, unknown>
      usage: { totalTokens: number }
    }>()
    expect(body.taskType).toBe("repo_config_infer")
    expect(body.output.buildDefinitionMode).toBe("repository_dockerfile")
    expect(body.usage.totalTokens).toBe(530)

    const usages = repository.recordedModelUsages()
    expect(usages).toHaveLength(1)
    expect(usages[0]?.ownerUserId).toBe("usr_dev")
    expect(usages[0]?.runId).toBe(body.runId)

    const convList = await repository.listConversations("usr_dev", 1, 20)
    expect(convList.total).toBe(0)
    await app.close()
  })

  it("fast-fails with 422 ai.wallet_balance_insufficient before calling the LLM when wallet is empty", async () => {
    const repository = new TestRepository()
    repository.setWalletBalance("usr_broke", 0)
    const rawProvider = new SequenceProvider([
      () => {
        throw new Error("provider should not be called when wallet is empty")
      },
    ])
    const provider = new UsageRecordingModelProvider(rawProvider, repository)
    const remote = testRemoteConfigSource(testRemoteProviderConfig())
    const headlessInference = new HeadlessInferenceRunner({
      repository,
      provider,
      remoteConfig: remote.source,
    })
    const app = buildTestServer({
      config: loadConfig({ NODE_ENV: "test" }),
      repository,
      requestVerifier: new DevelopmentRequestVerifier(),
      remoteConfig: remote.source,
      headlessInference,
    })

    const res = await app.inject({
      method: "POST",
      url: "/internal/v1/inference",
      headers: { "x-luna-dev-user": "usr_broke" },
      payload: {
        taskType: "delivery_failure_diagnosis",
        maxToolTier: "none",
        contextPayload: { failureCode: "job_failed" },
      },
    })

    expect(res.statusCode).toBe(422)
    expect(res.json()).toMatchObject({ error: { code: "ai.wallet_balance_insufficient" } })
    expect(rawProvider.requests).toHaveLength(0)
    await app.close()
  })

  it("allows L0 read-only tools and blocks L2 write execution while returning proposedAction", async () => {
    const repository = new TestRepository()
    repository.setWalletBalance("usr_ops", 50)
    const catalog = ToolCatalog.load([
      testToolOperation("runtime_get_target_diagnostics", {
        approvalTier: "L0",
        requiresApproval: false,
        effect: "read",
        method: "GET",
        path: "/api/v1/projects/{projectId}/diagnostics",
        inputSchema: {
          type: "object",
          properties: { projectId: { type: "string" } },
          required: ["projectId"],
          additionalProperties: false,
        },
      }),
      testToolOperation("delivery_restart_target", {
        approvalTier: "L2",
        requiresApproval: true,
        effect: "write",
        method: "POST",
        path: "/api/v1/projects/{projectId}/restart",
        inputSchema: {
          type: "object",
          properties: { projectId: { type: "string" }, target: { type: "string" } },
          required: ["projectId", "target"],
          additionalProperties: false,
        },
      }),
    ])
    const catalogRegistry = new ToolCatalogRegistry(catalog, "v1")
    const toolClient = new DeterministicLunaApiClient(() => ({
      status: 200,
      body: { oomCount: 3, lastExitCode: 137 },
    }))
    const rawProvider = new SequenceProvider([
      () => ({
        text: "",
        toolCalls: [
          {
            id: "call_details",
            operationId: "get_tool_details",
            arguments: { operationIds: ["runtime_get_target_diagnostics", "delivery_restart_target"] },
          },
        ],
        usage: { status: "reported", value: { inputTokens: 100, outputTokens: 30, totalTokens: 130 } },
      }),
      () => ({
        text: "",
        toolCalls: [
          {
            id: "call_l0",
            operationId: "runtime_get_target_diagnostics",
            arguments: { projectId: "prj_ops" },
          },
          {
            id: "call_l2_blocked",
            operationId: "delivery_restart_target",
            arguments: { projectId: "prj_ops", target: "dt_api" },
          },
        ],
        usage: { status: "reported", value: { inputTokens: 150, outputTokens: 40, totalTokens: 190 } },
      }),
      () => ({
        text: JSON.stringify({
          rootCauseSummary: "容器发生 OOMKilled（退出码 137）。",
          confidence: "high",
          suggestions: ["上调内存配额或先执行滚动重启恢复服务。"],
          proposedAction: {
            toolName: "delivery_restart_target",
            arguments: { projectId: "prj_ops", target: "dt_api" },
            summary: "滚动重启异常部署目标 dt_api",
          },
        }),
        usage: { status: "reported", value: { inputTokens: 200, outputTokens: 60, totalTokens: 260 } },
      }),
    ])

    const provider = new UsageRecordingModelProvider(rawProvider, repository)
    const remote = testRemoteConfigSource(testRemoteProviderConfig({ toolCatalog: catalog.all() }))
    const headlessInference = new HeadlessInferenceRunner({
      repository,
      provider,
      remoteConfig: remote.source,
      catalogRegistry,
      toolClient,
    })

    const app = buildTestServer({
      config: loadConfig({ NODE_ENV: "test" }),
      repository,
      requestVerifier: new DevelopmentRequestVerifier(),
      remoteConfig: remote.source,
      headlessInference,
    })

    const res = await app.inject({
      method: "POST",
      url: "/internal/v1/inference",
      headers: { "x-luna-dev-user": "usr_ops" },
      payload: {
        projectId: "prj_ops",
        taskType: "incident_diagnosis",
        maxToolTier: "L0",
        contextPayload: { anomalyType: "runtime.pod_oom_killed" },
      },
    })

    expect(res.statusCode).toBe(200)
    const body = res.json<{
      output: {
        rootCauseSummary: string
        proposedAction?: { toolName: string, arguments: Record<string, unknown> }
      }
      usage: { totalTokens: number }
    }>()
    expect(body.output.rootCauseSummary).toContain("OOMKilled")
    expect(body.output.proposedAction?.toolName).toBe("delivery_restart_target")
    expect(body.usage.totalTokens).toBe(580)

    // Only the L0 read tool reached the Go API client; the L2 write tool was blocked locally!
    expect(toolClient.headlessCalls).toHaveLength(1)
    expect(toolClient.headlessCalls[0]?.operation.operationId).toBe("runtime_get_target_diagnostics")
    await app.close()
  })

  it("rejects disabled users, enforces signed actor projectId boundary, and accepts rotated previous HMAC keys", async () => {
    const repository = new TestRepository()
    repository.setWalletBalance("usr_disabled", 100)
    repository.disabledUsers.add("usr_disabled")
    repository.setWalletBalance("usr_rotated", 100)

    const rawProvider = new SequenceProvider([
      () => ({
        text: JSON.stringify({
          summary: "Rotated key inference succeeded",
          confidence: "high",
          buildDefinitionMode: "repository_dockerfile",
        }),
        usage: { status: "reported", value: { inputTokens: 10, outputTokens: 5, totalTokens: 15 } },
      }),
    ])
    const provider = new UsageRecordingModelProvider(rawProvider, repository)
    const remote = testRemoteConfigSource(testRemoteProviderConfig())
    const headlessInference = new HeadlessInferenceRunner({
      repository,
      provider,
      remoteConfig: remote.source,
    })

    const verifier = new BffHmacRequestVerifier(
      "primary-service-token-32-bytes!!",
      "primary-signing-key-32-bytes-min",
      {
        serviceToken: "previous-service-token-32-bytes!",
        actorSigningKey: "previous-signing-key-32-bytes-mn",
      },
    )
    const app = buildTestServer({
      config: loadConfig({ NODE_ENV: "test" }),
      repository,
      requestVerifier: verifier,
      remoteConfig: remote.source,
      headlessInference,
    })

    const signHeaders = (actor: Record<string, unknown>, token: string, signingKey: string) => {
      const now = Math.floor(Date.now() / 1000)
      const encoded = Buffer.from(JSON.stringify({
        sessionId: "headless:test",
        locale: "zh-CN",
        requestId: "req_test",
        issuedAt: now,
        expiresAt: now + 60,
        ...actor,
      })).toString("base64url")
      const sig = `sha256=${createHmac("sha256", signingKey).update(encoded).digest("hex")}`
      return {
        authorization: `Bearer ${token}`,
        "x-luna-actor-context": encoded,
        "x-luna-actor-signature": sig,
      }
    }

    // 1. Disabled user fails closed (401 ai.unauthorized) even if actor.platformRole claims admin
    const disabledRes = await app.inject({
      method: "POST",
      url: "/internal/v1/inference",
      headers: signHeaders(
        { userId: "usr_disabled", platformRole: "admin" },
        "primary-service-token-32-bytes!!",
        "primary-signing-key-32-bytes-min",
      ),
      payload: { taskType: "repo_config_infer", contextPayload: {} },
    })
    expect(disabledRes.statusCode).toBe(401)
    expect(disabledRes.json()).toMatchObject({ error: { code: "ai.unauthorized" } })
    expect(rawProvider.requests).toHaveLength(0)

    // 2. Signed actor.projectId = prj_A cannot be overridden by payload projectId = prj_B (403 ai.project_scope_mismatch)
    const mismatchRes = await app.inject({
      method: "POST",
      url: "/internal/v1/inference",
      headers: signHeaders(
        { userId: "usr_rotated", projectId: "prj_A" },
        "primary-service-token-32-bytes!!",
        "primary-signing-key-32-bytes-min",
      ),
      payload: { projectId: "prj_B", taskType: "repo_config_infer", contextPayload: {} },
    })
    expect(mismatchRes.statusCode).toBe(403)
    expect(mismatchRes.json()).toMatchObject({ error: { code: "ai.project_scope_mismatch" } })

    // 3. Rotated previous HMAC key pair succeeds (200 OK)
    const rotatedRes = await app.inject({
      method: "POST",
      url: "/internal/v1/inference",
      headers: signHeaders(
        { userId: "usr_rotated", projectId: "prj_A" },
        "previous-service-token-32-bytes!",
        "previous-signing-key-32-bytes-mn",
      ),
      payload: { projectId: "prj_A", taskType: "repo_config_infer", contextPayload: {} },
    })
    expect(rotatedRes.statusCode).toBe(200)
    await app.close()
  })
})
