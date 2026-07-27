# Luna CLI

Luna CLI is the command-line client for Luna DevOps. It is designed for interactive terminal use and for automation agents that need stable JSON contracts.

Commands use a fixed two-level structure:

```text
luna <category> <tool> key=value
```

For example, machine-readable command discovery uses:

```bash
luna help catalog query=project limit=5 output=json interactive=false
```

## Current development status

The CLI is in prerelease and is runnable and testable. Its command catalog is
assembled from three controlled sources. Live totals come from
`luna help catalog` and the platform coverage gate; this documentation does not
maintain a second set of counts that can drift:

- ordinary business commands generated from OpenAPI, which is their sole source of truth;
- protocol adapters for transports such as SSE, WebSocket, and file downloads that cannot preserve their semantics through a normal JSON HTTP command;
- local commands for login, local configuration, help, and shell completion that do not map to a platform business API.

The source tree includes:

- one active server/account login and a default project;
- OAuth Device Code login by default, automatic refresh and best-effort revocation, plus an explicit personal-access-token fallback;
- `key=value`, JSON, file, and standard-input parameters;
- human-readable output and a versioned JSON envelope;
- local help, project, and completion command registration;
- human-friendly `login`, `logout`, `whoami`, and `doctor` root shortcuts;
- `health doctor` checks for the active login, authentication, server version, OpenAPI contract, and feature flags;
- automatic API generation, minimum CLI version, and OpenAPI digest checks before OpenAPI business commands;
- all ordinary business operations currently documented by OpenAPI;
- a shared npm/Bun entry point, packaging, global-install smoke tests, and release gates.

Shared contracts and the API client are bundled safely into npm and Bun artifacts, so users do not need the monorepo workspace. Prereleases are available through the npm `beta` channel.

## Built-in Help and locale selection

The CLI supports command discovery and basic operation without Skills:

```bash
luna --help
luna login
luna login server=https://devops.example.com
printf '%s' "$LUNA_TOKEN" | luna login mode=access-token token=@-
luna whoami
luna doctor
luna logout
luna project --help
luna project get-projects --help
```

The root shortcuts reuse the same handlers as `auth login`, `auth status`,
`health doctor`, and `auth logout`. They are for interactive human use only.
Scripts and Agents use canonical two-level commands, and `agent=true` rejects
root aliases.

Each level progressively exposes categories, tools, scopes, risk, parameters, input sources, and examples. Locale precedence is:

1. command-line `--lang`;
2. `LUNA_LANG`;
3. the configured `language`;
4. `LC_ALL`, `LC_MESSAGES`, `LANG`, and the runtime locale;
5. English fallback.

```bash
LUNA_LANG=zh-CN luna --help
luna --lang zh-CN project get-projects --help
```

See [Source Development and Verification](./development) for repository commands, OpenAPI regeneration, and validation.

## Design boundaries

- The CLI calls Luna DevOps backend APIs only. It does not orchestrate Kubernetes, GitHub, Gitea, or registry APIs directly.
- Ordinary HTTP business commands must be generated from OpenAPI. The CLI must not maintain a second handwritten route or parameter inventory.
- SSE, WebSocket, binary download, and authorized follow-up transports require explicit protocol adapters. An adapter must not duplicate an ordinary JSON HTTP API and must participate in the route-by-route coverage audit.
- Browser callbacks, external webhook receivers, and pre-application bootstrap endpoints are not business commands, but each exact `method + path` requires an audited classification and reason. Prefix-wide silent exclusions are not allowed.
- Automation should use `output=json interactive=false` and parse JSON from `stdout` only. Diagnostics belong on `stderr`.
- Local state defaults to `~/.luna/`. Tests and CI use a temporary `LUNA_HOME` and never read real user credentials.
- `high` and `critical` operations require per-operation confirmation in an
  interactive terminal. Non-interactive and Agent callers must pass `--yes`.
  `--yes` only suppresses the CLI prompt; it cannot bypass backend permissions,
  scopes, step-up MFA, or other server policy.
- CLI and platform versions are independent. Compatibility is negotiated through server capabilities rather than a version-string comparison alone.
- Canonical OpenAPI commands read `/api/v1/meta` on the first request to an
  instance and validate the API generation, minimum CLI version, and OpenAPI
  digest. Successful checks are cached within the process.
- `luna health doctor output=json` explicitly displays the active login, authentication,
  compatibility, and server feature diagnostics.
- A command in `help catalog` means that the current CLI registers it.
  `serverSupported` may still be `null`; the execution-time negotiation remains
  authoritative.
- Agents must explicitly pass
  `output=json interactive=false agent=true` to every command and must not rely
  on local output or interaction defaults. Agent mode also disables colors and
  applies safe pagination, polling, and response-size limits.
- After `luna project use project=<id>` sets a default project, project-scoped commands may omit a required
  `project`, `projectId`, or `projectID`; the CLI injects that immutable project ID without granting additional permissions.
- The CLI stores one active login only. A `luna login` without `server` always uses
  `https://devops.liteyuki.org`; explicitly signing in to another server or
  account replaces the stored credential and default project.
- `api request` is limited to human diagnostics against a known relative API path and is always disabled in Agent mode. It must not impersonate a business capability that is absent from OpenAPI or still requires a dedicated transport.
- Terminal and data-export operations use dedicated WebSocket/download protocol
  adapters. They require a CLI OAuth login and step-up MFA for the matching
  purpose; a personal access token cannot satisfy or bypass that requirement.

Machines and Agents discover capabilities through the machine catalog rather
than parsing human-oriented help:

```bash
luna help catalog all=true limit=100 output=json interactive=false agent=true
luna help command path=project.get-projects output=json interactive=false agent=true
```

Agent commands always use `output=json interactive=false agent=true`. Callers
parse the JSON envelope from `stdout` only and treat `stderr` as diagnostics.

## Agent Skill

The paired `luna-devops` Skill lives in
[`skills/luna-devops`](https://github.com/LiteyukiStudio/luna-cli/tree/main/skills/luna-devops)
in the standalone [`LiteyukiStudio/luna-cli`](https://github.com/LiteyukiStudio/luna-cli)
repository.
Its root `SKILL.md` defines intent routing, shared operation order, and safety
boundaries. Domain material lives under `references/` and is loaded only when
the task needs it. Machine-readable Help remains the source of truth for
commands, parameters, risk, and output.

The Skill ships with the CLI and must use the exact same version. Every
`v*` GitHub Release contains one `luna-devops-<version>.skill`; an Agent
must not load a mismatched version.

```bash
luna help catalog query=project limit=20 output=json interactive=false agent=true
luna help command path=project.get-projects output=json interactive=false agent=true
```

When commands, parameters, risks, or capability boundaries change, update the
Skill in the same change and run:

```bash
pnpm check:skills
```

## Remaining release blockers

Before the first stable release, the project must:

1. Keep the platform route, OpenAPI, CLI command, and protocol-adapter coverage gate passing; its output is the only source for coverage totals and ratios.
2. Add the CLI entry point for Authorization Code + PKCE; Device Code, refresh, revocation, and OAuth Bearer step-up MFA are available.
3. Complete the clean-instance all-operation and critical-journey validation,
   including terminals, data export, and step-up MFA.
4. Configure an npm Trusted Publisher and protect the GitHub `npm` Environment.
5. Add Apple Developer ID signing and notarization before macOS binaries enter stable releases; Windows continues to use npm/pnpm.

See [Install and Use](./installation), [Source Development and Verification](./development), and [Release Security](./release-security) for details.
