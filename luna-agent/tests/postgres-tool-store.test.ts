import type { Pool } from "pg"
import { describe, expect, it, vi } from "vitest"
import { PayloadCipher } from "../src/payload-cipher.js"
import type { Repository } from "../src/persistence/repository.js"
import { PostgresToolCallStore } from "../src/tools/postgres-store.js"

describe("Postgres tool argument storage", () => {
  it("stores only a redacted projection and restores executable arguments from ciphertext", async () => {
    const calls: unknown[][] = []
    const cipher = new PayloadCipher(Buffer.alloc(32, 9), "tool-arguments-test")
    const executable = {
      projectId: "prj_1",
      body: { username: "app", password: "generated-secret" },
    }
    const ciphertext = cipher.encrypt(JSON.stringify(executable))
    const query = vi.fn(async (sql: string, params?: unknown[]) => {
      calls.push(params ?? [])
      if (sql.startsWith("select *")) {
        return {
          rows: [{
            id: "aitool_1",
            run_id: "airun_1",
            operation_id: "installAppTemplate",
            status: "awaiting_approval",
            arguments: { projectId: "prj_1", body: { username: "app", password: "[REDACTED]" } },
            arguments_ciphertext: ciphertext,
            arguments_hash: "sha256:test",
            attempt: 1,
            row_version: 2,
            approval_expires_at: new Date(Date.now() + 60_000),
            result: null,
            error_code: null,
          }],
        }
      }
      return { rows: [] }
    })
    const store = new PostgresToolCallStore(
      { query } as unknown as Pool,
      {} as Repository,
      cipher,
    )

    await store.insert({
      id: "aitool_1",
      runId: "airun_1",
      operationId: "installAppTemplate",
      status: "proposed",
      arguments: executable,
      argumentsHash: "sha256:test",
      attempt: 1,
      rowVersion: 1,
      result: { code: "ai.tool_arguments_invalid", retryable: true },
      errorCode: "ai.tool_arguments_invalid",
    })

    expect(query.mock.calls[0]?.[0]).toContain("input_mode")
    expect(calls[0]?.[4]).toBe("model")
    expect(calls[0]?.[5]).toContain("[REDACTED]")
    expect(calls[0]?.[5]).not.toContain("generated-secret")
    expect(calls[0]?.[6]).not.toContain("generated-secret")
	expect(calls[0]?.[11]).toBe(JSON.stringify({ code: "ai.tool_arguments_invalid", retryable: true }))
	expect(calls[0]?.[12]).toBe("ai.tool_arguments_invalid")
    await expect(store.get("aitool_1")).resolves.toMatchObject({ arguments: executable })
  })

  it("never executes a legacy redacted projection when encrypted arguments are missing", async () => {
    const query = vi.fn(async () => ({
      rows: [{
        id: "aitool_legacy",
        run_id: "airun_legacy",
        operation_id: "installAppTemplate",
        status: "failed",
        arguments: { body: { password: "[REDACTED]" } },
        arguments_ciphertext: null,
        arguments_hash: "sha256:legacy",
        attempt: 1,
        row_version: 3,
        approval_expires_at: null,
        result: null,
        error_code: "ai.approval_arguments_changed",
      }],
    }))
    const store = new PostgresToolCallStore(
      { query } as unknown as Pool,
      {} as Repository,
      new PayloadCipher(Buffer.alloc(32, 9), "tool-arguments-test"),
    )

    await expect(store.get("aitool_legacy")).rejects.toThrow("ai.tool_arguments_key_unavailable")
  })

  it("maps a missing input_mode column to a stable schema error", async () => {
    const query = vi.fn(async () => {
      throw Object.assign(new Error("column input_mode does not exist"), { code: "42703" })
    })
    const store = new PostgresToolCallStore(
      { query } as unknown as Pool,
      {} as Repository,
      new PayloadCipher(Buffer.alloc(32, 9), "tool-arguments-test"),
    )

    await expect(store.insert({
      id: "aitool_schema",
      runId: "airun_schema",
      operationId: "listProjects",
      status: "proposed",
      arguments: {},
      argumentsHash: "sha256:schema",
      attempt: 1,
      rowVersion: 1,
    })).rejects.toThrow("ai.database_schema_mismatch")
  })
})
