import { describe, expect, it } from "vitest";

import {
  buildOperationCatalog,
  createFallbackOperationId,
  createOperationKey,
  filterOperationCatalog,
  findOperationByCommand,
  findOperationById,
  findOperationByRoute,
  normalizeOpenApiPath,
  OPERATION_CATALOG,
  OPERATION_CATALOG_METADATA,
  pageOperationCatalog,
} from "../src/catalog.js";
import {
  OPENAPI_OPERATION_SNAPSHOTS,
  OPENAPI_SNAPSHOT_METADATA,
} from "../src/generated/operations.js";
import type { OpenApiOperationSnapshot } from "../src/types.js";

function operation(
  overrides: Partial<OpenApiOperationSnapshot> = {},
): OpenApiOperationSnapshot {
  return {
    method: "get",
    path: "/api/v1/widgets/{widgetId}",
    tags: ["Widgets"],
    deprecated: false,
    security: [],
    parameters: [],
    responses: [],
    ...overrides,
  };
}

describe("OpenAPI operation catalog", () => {
  it("covers the committed OpenAPI snapshot with stable unique identifiers", () => {
    expect(OPERATION_CATALOG).toHaveLength(
      OPENAPI_SNAPSHOT_METADATA.operationCount,
    );
    expect(OPERATION_CATALOG).toHaveLength(
      OPENAPI_OPERATION_SNAPSHOTS.length,
    );
    expect(OPERATION_CATALOG_METADATA.operationCount).toBe(
      OPERATION_CATALOG.length,
    );

    expect(
      new Set(OPERATION_CATALOG.map(({ operationKey }) => operationKey)).size,
    ).toBe(OPERATION_CATALOG.length);
    expect(
      new Set(OPERATION_CATALOG.map(({ operationId }) => operationId)).size,
    ).toBe(OPERATION_CATALOG.length);
    expect(
      new Set(
        OPERATION_CATALOG.map(({ command }) => command.canonicalPath),
      ).size,
    ).toBe(OPERATION_CATALOG.length);

    for (const entry of OPERATION_CATALOG) {
      if (entry.operationIdSource === "fallback") {
        expect(entry.operationId).toBe(entry.fallbackOperationId);
        expect(entry.operationId).toMatch(/^fallback_/);
      } else {
        expect(entry.explicitOperationId).toBe(entry.operationId);
      }
    }
  });

  it("publishes expanded request input schemas for agent help", () => {
    const operation = OPERATION_CATALOG.find(
      ({ method, path }) =>
        method === "post" && path === "/api/v1/public/configs",
    );

    expect(operation?.inputSchema).toMatchObject({
      type: "object",
      required: ["body"],
      additionalProperties: false,
      properties: {
        body: {
          ref: "#/components/schemas/ConfigKeysInput",
          type: "object",
          required: ["keys"],
          properties: {
            keys: {
              type: "array",
              items: { type: "string" },
            },
          },
        },
      },
    });
  });

  it("normalizes route syntax and resolves all lookup forms", () => {
    expect(normalizeOpenApiPath("api/v1/projects/:projectId/")).toBe(
      "/api/v1/projects/{projectId}",
    );
    expect(
      createOperationKey("get", "api/v1/projects/:projectId/"),
    ).toBe("GET /api/v1/projects/{projectId}");

    const byRoute = findOperationByRoute(
      "get",
      "/api/v1/projects/:projectId",
    );
    expect(byRoute).toBeDefined();
    expect(findOperationById(byRoute!.operationId)).toBe(byRoute);
    expect(findOperationByCommand(byRoute!.command.canonicalPath)).toBe(
      byRoute,
    );
  });

  it("produces deterministic fallback metadata", () => {
    expect(
      createFallbackOperationId(
        "Projects",
        "get",
        "/api/v1/projects/{projectId}",
      ),
    ).toBe("fallback_projects_get_api_v1_projects_by_project_id");

    const [entry] = buildOperationCatalog([operation()]);
    expect(entry?.operationId).toBe(
      "fallback_widgets_get_api_v1_widgets_by_widget_id",
    );
    expect(entry?.operationIdSource).toBe("fallback");
    expect(entry?.command.source).toBe("fallback");
    expect(entry?.command.canonicalPath).toBe(
      "widgets.get-widgets-by-widget-id",
    );
  });

  it("adds semantic operation aliases to generated commands", () => {
    const [entry] = buildOperationCatalog([
      operation({
        operationId: "listProjectMembers",
        path: "/api/v1/projects/{projectId}/members",
        tags: ["Projects"],
      }),
    ]);

    expect(entry?.command.canonicalPath).toBe(
      "project.get-projects-by-project-id-members",
    );
    expect(entry?.command.aliases).toContain("list-project-members");
  });

  it("prefers explicit operation and command metadata when available", () => {
    const [entry] = buildOperationCatalog([
      operation({
        operationId: "getWidget",
        xLunaCli: {
          command: {
            category: "project",
            tool: "widget-show",
          },
          classification: "business-command",
          risk: "high",
          transport: "download",
          requiredScopes: ["project:read", "widget:read"],
          categoryAliases: ["workspace"],
          aliases: ["show-widget"],
          mfaPurpose: "data_export",
          projectContext: { mode: "required" },
          streaming: true,
          agentAllowed: false,
          examples: [
            "luna project widget-show projectId=prj_example widgetId=wid_example",
          ],
        },
      }),
    ]);

    expect(entry).toMatchObject({
      operationId: "getWidget",
      explicitOperationId: "getWidget",
      operationIdSource: "explicit",
      command: {
        canonicalPath: "project.widget-show",
        source: "explicit",
        classification: "business-command",
        risk: "high",
        transport: "download",
        requiredScopes: ["project:read", "widget:read"],
        categoryAliases: ["workspace"],
        aliases: ["show-widget", "get-widget"],
        mfaPurpose: "data_export",
        projectContext: "required",
        streaming: true,
        agentAllowed: false,
        examples: [
          "luna project widget-show projectId=prj_example widgetId=wid_example",
        ],
      },
    });
  });

  it("infers project context from project path parameters", () => {
    const [entry] = buildOperationCatalog([
      operation({
        parameters: [
          {
            name: "projectId",
            in: "path",
            required: true,
          },
        ],
      }),
    ]);

    expect(entry?.command.projectContext).toBe("required");
    expect(entry?.command.agentAllowed).toBe(true);
  });

  it("publishes explicit CLI contracts for dashboard and retention operations", () => {
    expect(findOperationByCommand("dashboard.show")?.command).toMatchObject({
      risk: "low",
      requiredScopes: ["dashboard:read"],
    });
    expect(findOperationByCommand("retention.catalog")?.command).toMatchObject({
      risk: "low",
      requiredScopes: ["retention:read"],
    });
    expect(findOperationByCommand("retention.preview")?.command).toMatchObject({
      risk: "low",
      requiredScopes: ["retention:read"],
    });
    expect(findOperationByCommand("retention.cleanup")?.command).toMatchObject({
      risk: "critical",
      requiredScopes: ["retention:manage"],
    });
    expect(findOperationByCommand("access-token.scope-list")?.command).toMatchObject({
      risk: "low",
      requiredScopes: ["token:manage"],
    });
  });

  it("filters and pages the catalog without changing source ordering", () => {
    const projectOperations = filterOperationCatalog({
      category: "project",
      method: "get",
    });
    expect(projectOperations.length).toBeGreaterThan(0);
    expect(
      projectOperations.every(
        ({ command, method }) =>
          command.category === "project" && method === "get",
      ),
    ).toBe(true);

    const queried = filterOperationCatalog({
      query: "topology",
      includeDeprecated: true,
    });
    expect(queried.length).toBeGreaterThan(0);
    expect(
      queried.every((entry) =>
        [
          entry.operationId,
          entry.operationKey,
          entry.command.canonicalPath,
          entry.summary,
          entry.description,
          ...entry.tags,
        ]
          .filter(Boolean)
          .join(" ")
          .toLowerCase()
          .includes("topology"),
      ),
    ).toBe(true);

    const visibleCatalog = filterOperationCatalog();
    const page = pageOperationCatalog({}, { offset: 2, limit: 3 });
    expect(page.items).toEqual(visibleCatalog.slice(2, 5));
    expect(page).toMatchObject({
      total: visibleCatalog.length,
      offset: 2,
      limit: 3,
      nextOffset: 5,
    });

    const completeCatalog = filterOperationCatalog({ includeHidden: true });
    expect(completeCatalog).toHaveLength(OPERATION_CATALOG.length);
    expect(
      completeCatalog.filter(
        (entry) =>
          entry.command.hidden &&
          entry.command.classification === "protocol-adapter",
      ),
    ).toHaveLength(14);
  });

  it("rejects duplicate public identifiers", () => {
    expect(() =>
      buildOperationCatalog([
        operation({ operationId: "duplicate" }),
        operation({
          operationId: "duplicate",
          method: "post",
          path: "/api/v1/widgets",
        }),
      ]),
    ).toThrow(/operation ID/i);
  });
});
