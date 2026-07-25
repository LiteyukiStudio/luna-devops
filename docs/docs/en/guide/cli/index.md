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

The CLI is in prerelease and is runnable and testable. Its current catalog contains 131 commands: 21 local commands and 110 commands generated from OpenAPI. The source tree includes:

- multi-instance contexts and a default project context;
- Access Token login, validation, and local credential storage;
- `key=value`, JSON, file, and standard-input parameters;
- human-readable output and a versioned JSON envelope;
- local help, context, project, and completion command registration;
- all 110 operations currently documented by OpenAPI;
- a shared npm/Bun entry point, packaging, global-install smoke tests, and release gates.

Shared contracts and the API client are bundled safely into npm and Bun artifacts, so users do not need the monorepo workspace. Prereleases are available through the npm `beta` channel.

## Built-in Help and locale selection

The CLI supports command discovery and basic operation without Skills:

```bash
luna --help
luna project --help
luna project get-projects --help
```

Each level progressively exposes categories, tools, scopes, risk, parameters, input sources, and examples. Locale precedence is:

1. command-line `--lang`;
2. `LUNA_LANG`;
3. the current context's `language`;
4. `LC_ALL`, `LC_MESSAGES`, `LANG`, and the runtime locale;
5. English fallback.

```bash
LUNA_LANG=zh-CN luna --help
luna --lang zh-CN project get-projects --help
```

See [Source Development and Verification](./development) for repository commands, OpenAPI regeneration, and validation.

## Design boundaries

- The CLI calls Luna DevOps backend APIs only. It does not orchestrate Kubernetes, GitHub, Gitea, or registry APIs directly.
- Automation should use `output=json interactive=false` and parse JSON from `stdout` only. Diagnostics belong on `stderr`.
- Local state defaults to `~/.luna/`. Tests and CI use a temporary `LUNA_HOME` and never read real user credentials.
- Medium-risk operations use the shared interactive confirmation flow. Non-interactive callers must set `yes=true`.
- High-risk API operations fail closed until the server-issued plan protocol exists; `yes=true` cannot bypass it.
- CLI and platform versions are independent. Compatibility is negotiated through server capabilities rather than a version-string comparison alone.
- A command in `help catalog` means that the current CLI registers it. Until capability negotiation is complete, `serverSupported` may still be `null`.
- Agents must pass `agent=true` to every command. This locks JSON output, disables interaction and colors, and applies safe pagination, polling, and response-size limits.
- When a context has a default project, project-scoped commands may omit a required
  `project`, `projectId`, or `projectID`; the CLI injects that immutable project ID without granting additional permissions.
- `api request` is limited to human diagnostics against a known relative API path and is always disabled in Agent mode. It must not impersonate a business capability that is absent from OpenAPI or still requires a dedicated transport.

## Agent Skills

The paired Skills live in the repository's [`ai-supports/skills`](https://github.com/LiteyukiStudio/luna-devops/tree/main/ai-supports/skills) directory. They define intent routing, operation order, and safety boundaries. Machine-readable Help remains the source of truth for commands, parameters, risk, and output.
The current prerelease pairing is Luna CLI `0.0.1-beta.1` with Luna CLI Skills
`0.1.0-beta.1`; the Skills require Luna CLI `>=0.0.1-beta.1 <0.1.0`.

```bash
luna help catalog query=project limit=20 agent=true
luna help command path=project.get-projects agent=true
```

After changing the command catalog or a capability boundary, update the Skills and run:

```bash
node scripts/cli/verify-skills-sync.mjs
```

## Remaining release blockers

Before the first stable release, the project must:

1. Document the remaining public backend routes in OpenAPI and complete command-coverage tests.
2. Add client capability negotiation, Authorization Code + PKCE, Device Code, and Bearer step-up MFA.
3. Complete SSE, WebSocket, download, and server-issued plan transports.
4. Configure an npm Trusted Publisher and protect the GitHub `npm` Environment.
5. Add Apple Developer ID signing and notarization before macOS binaries enter stable releases; Windows continues to use npm/pnpm.

See [Install and Use](./installation), [Source Development and Verification](./development), and [Release Security](./release-security) for details.
