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

## Scripts and Agents

Automation should use JSON output with interaction disabled:

```bash
luna project get-projects output=json interactive=false
```

High-risk operations still require explicit confirmation, and CLI never bypasses platform authorization. Pass tokens through environment variables or a secret manager instead of command history. Use `luna help` as the command and argument reference.
