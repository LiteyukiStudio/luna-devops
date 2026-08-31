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

## Scopes and project roles

The required scope shown by CLI help comes from the platform's published OpenAPI contract. Scopes issued by OAuth login are the credential's capability ceiling; they do not elevate the account's platform role or project-space role. For a regular project member, both the credential scope and project role must allow access; a platform administrator cannot bypass the credential scope either.

Logins to the same OAuth application from different devices or terminals form independent sessions. Signing out or revoking the current token affects only that session. Revoking the entire application authorization from account authorization settings invalidates every session for that application.

## kubectl kubeconfig

Use `luna kubeconfig write` to create a Kube Credential and atomically write a new file with `0600` permissions. Use `luna kubeconfig merge` to merge into an existing kubeconfig after conflict checks. These dedicated commands never print the one-time kubeconfig or token to normal stdout, and the current OAuth session must have `token:manage`.

See [Manage Project Resources with kubectl](/en/use/kubectl) for command options, safe merge behavior, context rules, and kubectl authorization boundaries.

## Scripts and Agents

Automation should use JSON output with interaction disabled:

```bash
luna project get-projects output=json interactive=false
```

High-risk operations still require explicit confirmation, and CLI never bypasses platform authorization. Pass tokens through environment variables or a secret manager instead of command history. Use `luna help` as the command and argument reference.
