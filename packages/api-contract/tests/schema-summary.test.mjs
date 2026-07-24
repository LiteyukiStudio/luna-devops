import { describe, expect, it } from "vitest";

import { createSchemaSummarizer } from "../scripts/schema-summary.mjs";

describe("OpenAPI schema summaries", () => {
  it("expands local component references into deterministic object fields", () => {
    const document = {
      components: {
        schemas: {
          Input: {
            type: "object",
            properties: {
              zeta: { type: "integer" },
              child: { $ref: "#/components/schemas/Child" },
              alpha: { type: "string" },
            },
            required: ["zeta", "alpha"],
          },
          Child: {
            type: "object",
            properties: {
              enabled: { type: "boolean" },
            },
            required: ["enabled"],
          },
        },
      },
    };

    expect(
      createSchemaSummarizer(document)(
        { $ref: "#/components/schemas/Input" },
      ),
    ).toEqual({
      ref: "#/components/schemas/Input",
      type: "object",
      required: ["alpha", "zeta"],
      properties: {
        alpha: { type: "string" },
        child: {
          ref: "#/components/schemas/Child",
          type: "object",
          required: ["enabled"],
          properties: {
            enabled: { type: "boolean" },
          },
        },
        zeta: { type: "integer" },
      },
    });
  });

  it("preserves references while protecting circular and deep schemas", () => {
    const document = {
      components: {
        schemas: {
          Node: {
            type: "object",
            properties: {
              next: { $ref: "#/components/schemas/Node" },
            },
          },
          LevelOne: {
            type: "object",
            properties: {
              child: { $ref: "#/components/schemas/LevelTwo" },
            },
          },
          LevelTwo: {
            type: "object",
            properties: {
              value: { type: "string" },
            },
          },
        },
      },
    };
    const summarize = createSchemaSummarizer(document, { maxDepth: 2 });

    expect(summarize({ $ref: "#/components/schemas/Node" })).toEqual({
      ref: "#/components/schemas/Node",
      type: "object",
      properties: {
        next: {
          ref: "#/components/schemas/Node",
          circular: true,
        },
      },
    });
    expect(summarize({ $ref: "#/components/schemas/LevelOne" })).toEqual({
      ref: "#/components/schemas/LevelOne",
      type: "object",
      properties: {
        child: {
          ref: "#/components/schemas/LevelTwo",
          truncated: true,
        },
      },
    });
    expect(summarize({ $ref: "https://example.com/schema.json" })).toEqual({
      ref: "https://example.com/schema.json",
    });
  });
});
