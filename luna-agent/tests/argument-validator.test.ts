import { describe, expect, it } from "vitest"
import { ToolArgumentsInvalidError, requiredInputFields, validateToolArguments } from "../src/tools/argument-validator.js"

describe("tool argument JSON Schema validation", () => {
  const schema = {
    type: "object",
    properties: {
      id: { type: "string", format: "uuid" },
      name: { type: "string", pattern: "^[a-z][a-z0-9-]+$", minLength: 3, maxLength: 12 },
      count: { type: "integer", minimum: 1, maximum: 5 },
      mode: { const: "safe" },
      tags: { type: "array", minItems: 1, maxItems: 3, uniqueItems: true, items: { type: "string", enum: ["build", "deploy"] } },
      config: {
        type: "object",
        properties: { enabled: { type: "boolean" } },
        required: ["enabled"],
        additionalProperties: false,
      },
      target: {
        oneOf: [
          { type: "object", properties: { kind: { const: "project" }, projectId: { type: "string", minLength: 1 } }, required: ["kind", "projectId"], additionalProperties: false },
          { type: "object", properties: { kind: { const: "cluster" }, clusterId: { type: "string", minLength: 1 } }, required: ["kind", "clusterId"], additionalProperties: false },
        ],
      },
      selector: { anyOf: [{ type: "string", minLength: 2 }, { type: "integer", minimum: 10 }] },
      marker: { allOf: [{ type: "string" }, { minLength: 3 }] },
    },
    required: ["id", "name", "count", "mode", "tags", "config", "target", "selector", "marker"],
    additionalProperties: false,
  }

  it("accepts nested values satisfying 2020-12 constraints", () => {
    const input = {
      id: "123e4567-e89b-12d3-a456-426614174000",
      name: "api-v1",
      count: 3,
      mode: "safe",
      tags: ["build", "deploy"],
      config: { enabled: true },
      target: { kind: "project", projectId: "prj_1" },
      selector: 10,
      marker: "yes",
    }
    expect(validateToolArguments(schema, input)).toBe(input)
  })

  it("returns field-level JSON Pointer issues for all supported constraints", () => {
    expect.assertions(8)
    try {
      validateToolArguments(schema, {
        id: "not-a-uuid",
        name: "A",
        count: 8,
        mode: "unsafe",
        tags: ["build", "build", "unknown", "deploy"],
        config: { enabled: "yes", extra: true },
        target: { kind: "missing" },
        selector: 1,
        marker: "x",
        undeclared: true,
      })
    }
    catch (error) {
      expect(error).toBeInstanceOf(ToolArgumentsInvalidError)
      const invalid = error as ToolArgumentsInvalidError
      expect(invalid.toJSON()).toMatchObject({ code: "ai.tool_arguments_invalid", retryable: true })
      expect(invalid.issues).toEqual(expect.arrayContaining([
        expect.objectContaining({ path: "/id", code: "format" }),
        expect.objectContaining({ path: "/name", code: "minLength" }),
        expect.objectContaining({ path: "/count", code: "maximum" }),
        expect.objectContaining({ path: "/mode", code: "const", allowedValues: ["safe"] }),
        expect.objectContaining({ path: "/tags", code: "maxItems" }),
        expect.objectContaining({ path: "/tags", code: "uniqueItems" }),
        expect.objectContaining({ path: "/tags/2", code: "enum", allowedValues: ["build", "deploy"] }),
        expect.objectContaining({ path: "/config/extra", code: "additionalProperties" }),
        expect.objectContaining({ path: "/config/enabled", code: "type" }),
        expect.objectContaining({ path: "/target", code: "oneOf" }),
        expect.objectContaining({ path: "/selector", code: "anyOf" }),
        expect.objectContaining({ path: "/marker", code: "minLength" }),
        expect.objectContaining({ path: "/undeclared", code: "additionalProperties" }),
      ]))
      expect(invalid.issues.every(issue => typeof issue.remediation === "string" && issue.remediation.length > 0)).toBe(true)
      expect(invalid.issues.find(issue => issue.path === "/mode" && issue.code === "const")?.allowedValues).toEqual(["safe"])
      expect(invalid.issues.find(issue => issue.path === "/tags/2" && issue.code === "enum")?.allowedValues).toEqual(["build", "deploy"])
      expect(invalid.issues.some(issue => issue.path === "/target" && issue.code === "oneOf")).toBe(true)
      expect(invalid.issues.some(issue => issue.path === "/selector" && issue.code === "anyOf")).toBe(true)
      return
    }
    throw new Error("expected validation to fail")
  })

  it("identifies pure missing input and escapes JSON Pointer tokens", () => {
    const missingSchema = {
      type: "object",
      properties: { "a/b~c": { type: "string" } },
      required: ["a/b~c"],
      additionalProperties: false,
    }
    try {
      validateToolArguments(missingSchema, {})
    }
    catch (error) {
      expect(error).toBeInstanceOf(ToolArgumentsInvalidError)
      const invalid = error as ToolArgumentsInvalidError
      expect(invalid.issues).toEqual([expect.objectContaining({ path: "/a~1b~0c", code: "required" })])
      expect(requiredInputFields(invalid)).toEqual(["a/b~c"])
      return
    }
    throw new Error("expected validation to fail")
  })

  it("does not classify invalid present values as missing user input", () => {
    try {
      validateToolArguments({ type: "object", properties: { value: { type: "string", minLength: 2 } }, required: ["value"], additionalProperties: false }, { value: "" })
    }
    catch (error) {
      expect(requiredInputFields(error as ToolArgumentsInvalidError)).toBeUndefined()
      return
    }
    throw new Error("expected validation to fail")
  })
})
