# Source Development

Luna CLI lives in the `cli/` workspace and reuses `packages/api-contract` and `packages/api-client`. Its catalog combines local commands with OpenAPI operations, so the CLI does not maintain a second handwritten API inventory.

## Run from source

Install the locked workspace dependencies from the repository root:

```bash
pnpm install --frozen-lockfile
```

Use a temporary home directory so development never reads real credentials:

```bash
export LUNA_HOME="$(mktemp -d)"
pnpm --silent --dir cli exec tsx src/entry.ts version show
pnpm --silent --dir cli exec tsx src/entry.ts help catalog query=project limit=10 output=json interactive=false
```

The normal location is `~/.luna`. Development, tests, and CI must use a temporary `LUNA_HOME`.

## Workspace responsibilities

| Path | Responsibility |
| --- | --- |
| `cli/src/commands` | Command registration, parsing, risk gates, and execution |
| `cli/src/auth` | Access Tokens, local credentials, and authentication status |
| `cli/src/config` | Active server, credential, and default project |
| `cli/src/input` | `key=value`, JSON, files, and standard input |
| `cli/src/output` | Human output, JSON envelopes, and redaction |
| `packages/api-contract` | Environment-neutral contracts generated from OpenAPI |
| `packages/api-client` | HTTP client shared by Web and CLI |
| `scripts/cli` | Contract checks, packaging, artifact verification, and release tools |
| `ai-supports/skills` | Agent Skills that operate through the CLI only |

## Add a platform command

Add public control-plane APIs to OpenAPI first, including a stable `operationId`, Scope, and `x-luna-cli` metadata. Generated operations are registered as:

```text
luna <category> <tool> key=value
```

Purely local capabilities such as authentication, projects, help, and completion are registered in `cli/src/commands/local.ts`. Do not create a parallel command implementation for an existing HTTP API.

After changing OpenAPI, run:

```bash
pnpm --filter @luna-devops/api-contract generate
node scripts/cli/verify-contract-drift.mjs
```

## Machine output and Skills

Automation and AI agents must set `agent=true`. Agent mode enforces JSON output, disables prompts and colors, and requires canonical command names:

```bash
luna help catalog query=registry limit=20 agent=true
luna help command path=registry.get-registries agent=true
```

Machine-readable Help is the source of truth for commands, parameters, risks, and output contracts. `ai-supports/skills` describes workflow and safety only. After changing commands or Skills, run:

```bash
node scripts/cli/verify-skills-sync.mjs
```

The check rejects unknown literal commands, Agent commands without `agent=true`, and known stale capability claims.

After `project use` defines a default project, the executor injects its immutable ID into
required `project`, `projectId`, or `projectID` parameters. Explicit command values
take precedence. Optional project parameters are not injected, which keeps global and
cross-project requests free from accidental filters.

`api request` is reserved for human diagnostics against a known relative API path.
Agent mode always rejects it and exposes no configuration or parameter bypass.

## Current boundaries

The source tree currently contains 14 local commands, one protocol command, and
110 OpenAPI commands. A Skill must not guess capabilities missing from the
machine-readable catalog or use `api request` to pretend that they are supported.

Release work still includes:

- public backend routes not yet documented in OpenAPI;
- server capability negotiation and compatibility gates;
- the CLI entry point for Authorization Code + PKCE;
- SSE, WebSocket, and binary download transports;
- server-issued short-lived plans for high-risk operations.

Remote high/critical commands fail closed until server plans exist. `yes=true` cannot bypass this gate.

## Validation

```bash
pnpm --filter @liteyuki/luna-cli typecheck
pnpm --filter @liteyuki/luna-cli lint
pnpm --filter @liteyuki/luna-cli test
pnpm --filter @liteyuki/luna-cli build
node --test scripts/cli/tests/*.test.mjs
node scripts/cli/verify-skills-sync.mjs
```

See `notes/cli-spec.md` and `TODO.md` in the repository for the full design and remaining work.
