import { describe, expect, it } from "vitest"
import { createOptionsInput, fallbackOptionsInput, optionUIActions } from "../src/tools/ui-options.js"
import { automaticRouteUIAction, navigateToRouteInput } from "../src/tools/ui-route.js"

describe("create options tool", () => {
  it("maps validated model choices to the three supported UI action types", () => {
    const input = createOptionsInput.parse({
      title: "Choose the next step",
      options: [
        { id: "projects", label: "Open projects", action: { type: "navigate", routeName: "projects" } },
        { id: "explain", label: "Explain the failure", action: { type: "send_message", message: "Explain the latest failure" } },
        { id: "retry", label: "Retry build", action: { type: "request_tool", operationId: "retryBuildRun", arguments: { runId: "run_1" }, message: "Retry build run_1" } },
      ],
    })

    expect(optionUIActions(input)).toEqual([
      expect.objectContaining({ id: "projects", repeatable: true, type: "navigate", payload: { routeName: "projects", params: {}, query: {} } }),
      expect.objectContaining({ id: "explain", repeatable: false, type: "send_message", payload: { message: "Explain the latest failure" } }),
      expect.objectContaining({ id: "retry", repeatable: false, type: "request_tool", payload: { operationId: "retryBuildRun", arguments: { runId: "run_1" }, message: "Retry build run_1" } }),
    ])
  })

  it("rejects arbitrary URLs, malformed payloads, and repeatable non-idempotent actions", () => {
    expect(createOptionsInput.safeParse({
      title: "Unsafe",
      options: [
        { id: "external", label: "Open external", action: { type: "navigate", routeName: "https://evil.example" } },
        { id: "continue", label: "Continue", action: { type: "send_message", message: "Continue" } },
      ],
    }).success).toBe(false)
    expect(createOptionsInput.safeParse({
      title: "Duplicate IDs",
      options: [
        { id: "same", label: "Projects", action: { type: "navigate", routeName: "projects" } },
        { id: "same", label: "Dashboard", action: { type: "navigate", routeName: "dashboard" } },
      ],
    }).success).toBe(false)
    expect(createOptionsInput.safeParse({
      title: "Unsafe repeat",
      options: [
        { id: "repeat-message", label: "Repeat message", repeatable: true, action: { type: "send_message", message: "Run again" } },
        { id: "projects", label: "Projects", action: { type: "navigate", routeName: "projects" } },
      ],
    }).success).toBe(false)
    expect(createOptionsInput.safeParse({
      title: "Too many",
      options: Array.from({ length: 6 }, (_, index) => ({
        id: `option-${index}`,
        label: `Option ${index}`,
        action: { type: "send_message", message: `Choose ${index}` },
      })),
    }).success).toBe(false)
  })

  it("creates a safe localized fallback with 2-5 actions", () => {
    const fallback = fallbackOptionsInput({
      locale: "zh-CN",
      routeName: "application.detail",
      projectId: "prj_1",
      applicationId: "app_1",
    })

    expect(fallback.options).toHaveLength(3)
    expect(fallback.options.every(option => option.action.type === "send_message")).toBe(true)
    expect(fallback.options[0]?.action).toEqual({
      type: "send_message",
      message: "请继续分析当前应用（项目空间 ID：prj_1，应用 ID：app_1），并给出下一步建议。",
    })
  })

  it("creates a one-shot automatic action only for a registered frontend route", () => {
    const input = navigateToRouteInput.parse({ routeName: "project.workspace", params: { projectId: "prj_1" } })
    expect(automaticRouteUIAction(input)).toEqual({
      version: 1,
      type: "navigate",
      activation: "automatic",
      repeatable: false,
      payload: {
        routeName: "project.workspace",
        params: { projectId: "prj_1" },
        query: {},
      },
    })
    expect(navigateToRouteInput.safeParse({ routeName: "https://evil.example" }).success).toBe(false)
  })
})
