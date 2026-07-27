import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

import {
  NON_COMMAND_ROUTE_CLASSIFICATIONS,
  evaluateCoverage,
  extractGinRoutes,
  inspectCliExecutionPolicy,
  parseOpenApiSource,
} from "../verify-platform-cli-coverage.mjs";

const fixtureRoot = join(
  dirname(fileURLToPath(import.meta.url)),
  "fixtures",
  "platform-cli-coverage",
);
const openApiOperations = parseOpenApiSource(
  readFileSync(join(fixtureRoot, "openapi.yaml"), "utf8"),
  "fixture/openapi.yaml",
);
const cliCommands = JSON.parse(
  readFileSync(join(fixtureRoot, "catalog.json"), "utf8"),
);
const classifications = Object.freeze({
  "POST /api/v1/oauth/device/authorization": Object.freeze({
    classification: "protocol-adapter",
    reason: "Consumed by the fixture login protocol adapter.",
  }),
  "GET /api/v1/auth/oidc/{providerId}/start": Object.freeze({
    classification: "browser-callback",
    reason: "Starts the fixture browser authorization redirect.",
  }),
  "POST /api/v1/git/webhooks/{bindingId}": Object.freeze({
    classification: "webhook-receiver",
    reason: "Receives signed fixture webhook events.",
  }),
});

test("passes executable commands and exact audited non-command endpoints", () => {
  const routes = extractGinRoutes(
    readFileSync(join(fixtureRoot, "router-covered.txt"), "utf8"),
    "fixture/router-covered.txt",
  );
  const result = evaluateCoverage({
    routes,
    openApiOperations,
    cliCommands,
    classifications,
  });

  assert.equal(result.ok, true, result.errors.join("\n"));
  assert.equal(result.totals.platform, 5);
  assert.equal(result.totals.openapi, 5);
  assert.equal(result.totals.registered, 2);
  assert.equal(result.totals.executable, 2);
  assert.equal(result.totals.excluded, 3);
  assert.equal(result.totals.businessRoutes, 2);
  assert.equal(result.totals.registeredBusinessCoverage, 100);
  assert.equal(result.totals.businessCoverage, 100);
  assert.deepEqual(
    result.rows.map(route => route.classification),
    [
      "command",
      "protocol-adapter",
      "browser-callback",
      "webhook-receiver",
      "command",
    ],
  );
});

test("counts public configs as a business command", () => {
  const routes = extractGinRoutes(
    `func register(router *gin.Engine) {
      v1 := router.Group("/api/v1")
      v1.POST("/public/configs", handlers.PublicConfigs)
    }`,
    "fixture/router-public-configs.go",
  );
  const result = evaluateCoverage({
    routes,
    openApiOperations,
    cliCommands,
    classifications: {},
  });

  assert.equal(result.ok, true, result.errors.join("\n"));
  assert.equal(result.totals.businessRoutes, 1);
  assert.equal(result.totals.registered, 1);
  assert.equal(result.totals.executable, 1);
});

test("fails when a new business route has no OpenAPI command", () => {
  const routes = extractGinRoutes(
    readFileSync(join(fixtureRoot, "router-unknown.txt"), "utf8"),
    "fixture/router-unknown.txt",
  );
  const result = evaluateCoverage({
    routes,
    openApiOperations,
    cliCommands,
    classifications: {},
  });

  assert.equal(result.ok, false);
  assert.match(
    result.errors.join("\n"),
    /GET \/api\/v1\/projects\/\{projectId\}\/new-business-capability: business route is missing from OpenAPI/,
  );
  assert.equal(result.totals.businessCoverage, 50);
});

test("does not let an exact protocol classification swallow a neighboring route", () => {
  const routes = extractGinRoutes(
    `func register(router *gin.Engine) {
      v1 := router.Group("/api/v1")
      v1.POST("/oauth/device/authorization/extra", handlers.Unexpected)
    }`,
    "fixture/router-neighbor.go",
  );
  const result = evaluateCoverage({
    routes,
    openApiOperations,
    cliCommands,
    classifications,
    requireAllClassificationEntries: false,
  });

  assert.equal(result.ok, false);
  assert.match(
    result.errors.join("\n"),
    /POST \/api\/v1\/oauth\/device\/authorization\/extra: business route is missing from OpenAPI/,
  );
});

test("does not count a hidden raw command as business coverage", () => {
  const routes = extractGinRoutes(
    `func register(router *gin.Engine) {
      v1 := router.Group("/api/v1")
      v1.GET("/projects", handlers.ListProjects)
    }`,
    "fixture/router-hidden-command.go",
  );
  const result = evaluateCoverage({
    routes,
    openApiOperations,
    cliCommands: cliCommands.map(command =>
      command.path === "project.list"
        ? { ...command, hidden: true }
        : command),
    classifications: {},
  });

  assert.equal(result.ok, false);
  assert.equal(result.totals.registered, 0);
  assert.equal(result.totals.executable, 0);
  assert.match(
    result.errors.join("\n"),
    /command "project\.list" is hidden and cannot satisfy business coverage/,
  );
});

test("does not count a coincidental non-OpenAPI command registration", () => {
  const routes = extractGinRoutes(
    `func register(router *gin.Engine) {
      v1 := router.Group("/api/v1")
      v1.GET("/projects", handlers.ListProjects)
    }`,
    "fixture/router-wrong-source.go",
  );
  const result = evaluateCoverage({
    routes,
    openApiOperations,
    cliCommands: cliCommands.map(command =>
      command.path === "project.list"
        ? { ...command, source: "local" }
        : command),
    classifications: {},
  });

  assert.equal(result.ok, false);
  assert.equal(result.totals.registered, 0);
  assert.match(
    result.errors.join("\n"),
    /registered from "local", expected "openapi"/,
  );
});

test("distinguishes registered commands from server-supported commands", () => {
  const routes = extractGinRoutes(
    `func register(router *gin.Engine) {
      v1 := router.Group("/api/v1")
      v1.GET("/projects", handlers.ListProjects)
    }`,
    "fixture/router-unsupported-command.go",
  );
  const result = evaluateCoverage({
    routes,
    openApiOperations,
    cliCommands: cliCommands.map(command =>
      command.path === "project.list"
        ? { ...command, serverSupported: false }
        : command),
    classifications: {},
  });

  assert.equal(result.ok, false);
  assert.equal(result.totals.registered, 1);
  assert.equal(result.totals.executable, 0);
  assert.match(
    result.errors.join("\n"),
    /registered but marked unsupported by the server/,
  );
});

test("detects a global risk policy hard blocker", () => {
  const executionPolicy = inspectCliExecutionPolicy(
    `async function enforceRiskPolicy(registered) {
      if (registered.metadata.risk === "high" || registered.metadata.risk === "critical") {
        throw new CliCommandError("server_plan_required", "not executable")
      }
      throw new CliCommandError("confirmation_required", "pass --yes")
    }`,
    "fixture/executor-blocked.ts",
  );
  const routes = extractGinRoutes(
    `func register(router *gin.Engine) {
      v1 := router.Group("/api/v1")
      v1.GET("/projects", handlers.ListProjects)
    }`,
    "fixture/router-blocked-command.go",
  );
  const result = evaluateCoverage({
    routes,
    openApiOperations,
    cliCommands: cliCommands.map(command =>
      command.path === "project.list"
        ? { ...command, risk: "high" }
        : command),
    classifications: {},
    executionPolicy,
  });

  assert.deepEqual(executionPolicy.blockedRisks, ["high", "critical"]);
  assert.equal(result.ok, false);
  assert.equal(result.totals.registered, 1);
  assert.equal(result.totals.executable, 0);
  assert.match(
    result.errors.join("\n"),
    /global CLI policy "server_plan_required" hard-blocks high\/critical commands/,
  );
  assert.match(
    result.errors.join("\n"),
    /command "project\.list" is registered but globally blocked at risk "high"/,
  );
});

test("allows confirmation and cancellation outcomes in the risk policy", () => {
  const executionPolicy = inspectCliExecutionPolicy(
    `async function enforceRiskPolicy() {
      throw new CliCommandError("confirmation_required", "pass --yes")
      throw new CliCommandError("operation_cancelled", "cancelled")
    }`,
    "fixture/executor-confirmation.ts",
  );

  assert.deepEqual(executionPolicy.hardBlockers, []);
  assert.deepEqual(executionPolicy.blockedRisks, []);
});

test("rejects broad explicit exclusions", () => {
  const routes = extractGinRoutes(
    `func register(router *gin.Engine) {
      v1 := router.Group("/api/v1")
      v1.POST("/public/configs", handlers.PublicConfigs)
    }`,
    "fixture/router-explicit-exclusion.go",
  );
  const result = evaluateCoverage({
    routes,
    openApiOperations,
    cliCommands,
    classifications: {
      "POST /api/v1/public/configs": {
        classification: "explicit-exclusion",
        reason: "Broad browser bootstrap exclusion.",
      },
    },
  });

  assert.equal(result.ok, false);
  assert.match(
    result.errors.join("\n"),
    /invalid non-command classification "explicit-exclusion"/,
  );
});

test("requires audited exclusions to be hidden in OpenAPI", () => {
  const routes = extractGinRoutes(
    `func register(router *gin.Engine) {
      v1 := router.Group("/api/v1")
      v1.GET("/auth/oidc/:providerId/start", handlers.StartOIDC)
    }`,
    "fixture/router-visible-callback.go",
  );
  const operations = openApiOperations.map(operation =>
    operation.key === "GET /api/v1/auth/oidc/{providerId}/start"
      ? { ...operation, hidden: false }
      : operation);
  const result = evaluateCoverage({
    routes,
    openApiOperations: operations,
    cliCommands,
    classifications: {
      "GET /api/v1/auth/oidc/{providerId}/start":
        classifications["GET /api/v1/auth/oidc/{providerId}/start"],
    },
  });

  assert.equal(result.ok, false);
  assert.match(
    result.errors.join("\n"),
    /audited non-command route must be hidden and include an OpenAPI exclusion reason/,
  );
});

test("uses the exact audit classification for browser verification UI", () => {
  const routes = extractGinRoutes(
    `func register(router *gin.Engine) {
      v1 := router.Group("/api/v1")
      v1.GET("/oauth/device/verification", handlers.DeviceVerificationPage)
      v1.POST("/oauth/device/verification", handlers.SubmitDeviceVerification)
    }`,
    "fixture/router-browser-classification.go",
  );
  const operations = routes.map(route => ({
    key: route.key,
    method: route.method,
    path: route.path,
    commandPath: `auth.${route.method.toLowerCase()}DeviceVerification`,
    classification: "protocol-adapter",
    hidden: true,
    exclusionReason: "Implemented as an interactive browser verification UI.",
    risk: "low",
  }));
  const result = evaluateCoverage({
    routes,
    openApiOperations: operations,
    cliCommands,
    classifications: {
      "GET /api/v1/oauth/device/verification":
        NON_COMMAND_ROUTE_CLASSIFICATIONS["GET /api/v1/oauth/device/verification"],
      "POST /api/v1/oauth/device/verification":
        NON_COMMAND_ROUTE_CLASSIFICATIONS["POST /api/v1/oauth/device/verification"],
    },
  });

  assert.equal(result.ok, true, result.errors.join("\n"));
  assert.deepEqual(
    result.rows.map(route => route.classification),
    ["browser-callback", "browser-callback"],
  );
  assert.equal(result.totals.excluded, 2);
});
