# Use Luna CLI

Luna CLI manages Luna DevOps from a terminal. If it is not installed yet, start with [Install Luna CLI](./installation).

## Command format

Business commands use one consistent format:

```text
luna <category> <operation> key=value
```

For example:

```bash
luna project get-projects
luna project use project=prj_example
```

After selecting a default project space, project-level commands can omit its ID.

## Find a command

You do not need to memorize the command catalog. Browse help one level at a time:

```bash
luna --help
luna project --help
luna project get-projects --help
```

Help includes parameters, required permissions, risk level, and examples.

## Common tasks

```bash
luna whoami
luna doctor
luna project get-projects
luna logout
```

Run `luna doctor` first when troubleshooting connectivity, authentication, or version compatibility.

## Scripts and automation

Use machine-readable output in scripts:

```bash
luna project get-projects output=json interactive=false
```

Parse only JSON from standard output. Do not depend on table widths, colors, or localized prose. High-risk operations ask for confirmation in an interactive terminal; non-interactive environments must pass `--yes` explicitly, but it never bypasses platform permissions or security checks.

When signing in with a personal token, read it from an environment variable or secret manager instead of placing it in shell history:

```bash
printf '%s' "$LUNA_TOKEN" | luna login mode=access-token token=@-
```

## Agent use

Agents should use:

```text
output=json interactive=false agent=true
```

The companion `luna-devops` Skill is available from the [Luna CLI repository](https://github.com/LiteyukiStudio/luna-cli/tree/main/skills/luna-devops). Use `luna help` as the source of truth for commands and parameters.
