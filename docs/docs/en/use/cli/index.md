# Use Luna CLI

Luna CLI manages Luna DevOps from a terminal or automation script. Read [Install](./installation) first if needed.

## Command format

```text
luna <category> <operation> key=value
```

For example:

```bash
luna project get-projects
luna project use project=prj_example
```

List commands default to `visibility=related`. Platform administrators pass `visibility=all` only when they explicitly need platform-wide results, for example `luna project get-projects visibility=all`. Resource queries with a known project space should still pass its project ID to narrow the result.

Use layered help to find arguments, permissions, and examples:

```bash
luna --help
luna project --help
luna project get-projects --help
```

Run `luna doctor` first for connectivity, authentication, or version problems.

## Open an interactive release terminal

Sign in with OAuth, then attach the local terminal to a running release container:

```bash
luna release exec projectId=prj_example releaseId=rel_example
luna release exec projectId=prj_example releaseId=rel_example container=api
```

Once connected, the local terminal is attached to the shell in the Release's
current workload container. Run `exit` or press `Ctrl-D` to end the remote session
and restore the local terminal. `release terminal` is an alias for the same human
command.

The command requires an interactive TTY, the `deployment:exec` scope, a permitted
project role, and runtime-terminal access enabled for the project space and
deployment target. It does not expose cluster credentials and cannot run in a
script or Agent mode.

## Scopes and project roles

The required scope shown by CLI help comes from the platform's published OpenAPI contract. Scopes issued by OAuth login are the credential's capability ceiling; they do not elevate the account's platform role or project-space role. For a regular project member, both the credential scope and project role must allow access; a platform administrator cannot bypass the credential scope either.

Logins to the same OAuth application from different devices or terminals form independent sessions. Signing out or revoking the current token affects only that session. Revoking the entire application authorization from account authorization settings invalidates every session for that application.

## Scripts and Agents

Automation should use JSON output with interaction disabled:

```bash
luna project get-projects output=json interactive=false
```

High-risk operations still require explicit confirmation, and CLI never bypasses platform authorization. Pass tokens through environment variables or a secret manager instead of command history. Use `luna help` as the command and argument reference.
