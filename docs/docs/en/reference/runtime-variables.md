# Runtime Environment Variables and References

This page covers naming rules, merge precedence, `${NAME}` reference syntax, generated secrets, and file path rules for environment variables in deploy configs and shared runtime configs. For the steps, see [Configuration and Secrets](../use/workflow/configuration-and-secrets).

## Name and value limits

| Item | Rule |
| --- | --- |
| Variable name | Letters, digits, and underscores; cannot start with a digit; at most 128 characters, for example `DATABASE_URL` or `_TOKEN` |
| Variable value | At most 8192 characters |
| Count | At most 128 variables per deploy config or shared runtime config |
| Uniqueness | Within one deploy config or shared runtime config, a plain variable and an encrypted variable cannot share a name |

Invalid input fails to save. Common error codes: `deployment.runtime_environment_key_invalid` (invalid name), `deployment.runtime_environment_key_duplicate` (duplicate name), `deployment.runtime_environment_key_conflict` (plain and encrypted share a name), and `deployment.runtime_environment_value_too_long` (value too long).

## Merge precedence

At release time, values are merged in this order; later layers override earlier values with the same name:

1. Shared runtime configs referenced by the deploy config, in reference order.
2. The deploy config's own environment variables.

An encrypted variable at any layer wins over a plain variable with the same name at another layer. Variables injected by service relations are added after the merge. If a service relation variable has the same name as an existing variable, the release fails; rename one of them.

## `${NAME}` references

A plain variable value can reference another variable effective in the same deployment with `${NAME}`. References are expanded at release time:

```text
DB_HOST=postgres
DB_PORT=5432
DB_NAME=app
DATABASE_URL=postgresql://app@${DB_HOST}:${DB_PORT}/${DB_NAME}
```

After release, `DATABASE_URL` in the container is `postgresql://app@postgres:5432/app`.

Rules:

- Only the `${NAME}` form is supported, and `NAME` must follow the variable name rules. `$NAME` and `$(NAME)` are not expanded by the platform.
- You can reference merged plain variables, encrypted variables, and variables injected by service relations, for example `${POSTGRES_HOST}`.
- Chained references work (A references B, and B references C).
- A reference to a missing variable or to itself is kept as is; `${MISSING}` stays `${MISSING}`.
- A reference to a variable with an empty value is replaced with an empty string.
- Only plain variable values are expanded. Encrypted values, configuration files, and secret files are not.

> **Note**: when a plain variable references an encrypted variable, the fully expanded value, secret included, is written to the ConfigMap that holds plain variables, where authorized clients can read it. Store connection strings that contain passwords as a single encrypted variable instead of composing them from plain variables.

## Generated encrypted values

When you choose **Regenerate** for an encrypted variable in the environment table, the platform generates a 32-character random value from the Base64 character set (`A-Z a-z 0-9 + /`) on save. The value is never shown.

When generating through the API you can set:

| Parameter | Allowed values | Default |
| --- | --- | --- |
| Length | 8–256 | 32 |
| Encoding | `base64`, `hex`, `alphanumeric`, `numeric` | `base64` |

For random values that go into a URL, prefer `alphanumeric` or `hex` so `+` and `/` need no escaping. Invalid parameters return `deployment.secret_generation_invalid`.

## Configuration and secret file paths

- The path must be an absolute path in the container, for example `/app/config/settings.yaml`, and cannot be `/`.
- Paths are normalized, so `/app/./config/../settings.yaml` equals `/app/settings.yaml`. Normalized paths must be unique.
- Files are mounted read-only and cannot conflict with data volume mount paths.
- When a shared runtime config and the deploy config define the same path, the deploy config wins.
- File content is mounted as is; variables are not rendered.

An invalid path returns `deployment.runtime_config_path_invalid`; a conflict with a data volume returns `deployment.runtime_path_invalid`.

## When changes take effect

Changes to environment variables, shared runtime configs, or files affect running Pods only after a new release. When a shared runtime config attached as a **Live ref** changes, the platform prompts you to redeploy; a **Snapshot** copies the content at reference time and ignores later updates.
