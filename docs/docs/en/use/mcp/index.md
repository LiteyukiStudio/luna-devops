# MCP Integration

Luna DevOps provides a built-in Streamable HTTP MCP Server at `https://<instance>/api/v1/mcp`. Connect Claude Code, Cursor, Antigravity, Windsurf, or any Streamable HTTP MCP client directly to query resources, diagnose failures, and execute governed delivery operations.

`/api/v1/mcp` remains available on self-hosted instances even when the built-in Web AI Assistant is disabled (`AI_ASSISTANT_AVAILABLE=false`), because your external client uses its own model runtime.

## Prerequisites and authentication

- A reachable HTTPS URL for the instance (`localhost` HTTP is allowed only for local development).
- One of the following Bearer credentials; every call enforces both token Scopes and your account's real-time platform and project RBAC:

### 1. Personal Access Token (PAT, fastest)

Open **Account settings → Personal tokens** (Personal Access Tokens) in the console, select least-privilege Scopes (for example read-only Scopes for `L0` diagnostics, adding write Scopes only when deployment changes are required), create a token (`lyd_...`), and pass it in the request header:

```text
Authorization: Bearer <YOUR_PAT>
```

### 2. OAuth 2.0 application

Open **Account settings → Developer apps** (OAuth Applications) to register an application with an exact Redirect URI (HTTPS in production; `localhost` HTTP allowed for local development) and Allowed Scopes. Standards-compliant MCP clients automatically discover endpoints from the metadata URLs below and complete `authorization_code` + PKCE (`S256`) browser authorization:

- Protected resource metadata: `/.well-known/oauth-protected-resource`
- Authorization server metadata: `/.well-known/oauth-authorization-server`

## Shortest configuration steps

Replace `https://devops.example.com` with your instance URL and `<YOUR_PAT>` with your `lyd_...` token.

### Claude Code

```bash
claude mcp add --transport http luna-devops https://devops.example.com/api/v1/mcp \
  --header "Authorization: Bearer <YOUR_PAT>"
```

### Cursor / Antigravity / Standard `mcpServers` JSON

Add the server to your client's MCP configuration file:

```json
{
  "mcpServers": {
    "luna-devops": {
      "url": "https://devops.example.com/api/v1/mcp",
      "headers": {
        "Authorization": "Bearer <YOUR_PAT>"
      }
    }
  }
}
```

### Expected result

Once saved, your client's MCP panel shows `luna-devops` connected with 7 progressive meta-tools. Verify access by asking a read-only question such as “List the project spaces I can access.” See [Companion Skills](./skills) to load domain-specific workflows.

## Progressive tool discovery and safety model

Instead of loading hundreds of schemas into context upfront, the MCP Server exposes **166 domain operations** across 7 domains through **7 built-in progressive meta-tools**:

| Meta-tool | Purpose |
| --- | --- |
| `search_tools` | Search candidate platform operations using focused single-domain keywords. |
| `get_tool_details` | Inspect the full input Schema, risk tier (`L0` / `L1` / `L2`), and required token Scopes before calling. |
| `preview_tool` | Run a read-only dry-run for `L2` changes, returning a redacted `before -> after` Diff, warnings, and a signed `previewLock`. |
| `execute_tool` | Execute a domain operation under token Scope and real-time RBAC checks. |
| `load_skill` | Load domain workflow instructions on demand; see [Companion Skills](./skills). |
| `search_docs` | Search the official public documentation bundled with the running server version. |
| `read_doc` | Read official documentation sections to verify field formats, template variables, and limits. |

### Three-tier risk gate and mandatory `L2` preview lock

Every operation follows a three-tier safety policy:

- **`L0` (Read-only & diagnostics)**: Call `execute_tool(toolName, arguments)` directly.
- **`L1` (Reversible metadata & personal preferences)**: Call `execute_tool(toolName, arguments, confirmed: true)` after verifying arguments.
- **`L2` (High-risk & production changes — mandatory two-step lock)**: Covers production (`stage=prod`), deploy config specs/env/secrets/volume mounts, triggering builds or releases, scaling/restarting, access routes, member roles, resource deletion, and platform administration:
  1. Call `preview_tool(toolName, arguments)` to compute the redacted `before -> after` Diff and obtain a `previewLock` bound to the exact arguments and resource state;
  2. Present the target and Diff to the user and obtain explicit confirmation;
  3. Call `execute_tool(toolName, arguments, previewLock)` with identical arguments. Concurrent modifications between preview and execution are rejected to prevent overwriting newer state (TOCTOU protection).

### Operations reserved for the Web Console

For credential isolation or streaming protocol reasons, complete the following tasks in the browser console rather than through MCP:

- Issuing Personal Access Tokens (PATs), changing sign-in passwords, managing OAuth applications, and completing browser-based Git / OIDC account binding;
- Interactive container WebSocket TTY shells (AI clients can only run single-shot non-TTY commands when runtime terminal access is enabled);
- Uploading local archives for volume import and downloading exported volume archives;
- Advanced Kubernetes scheduling fields on deploy configs (`initContainers`, `sidecarContainers`, `tolerations`, `affinity`, etc.) and the cross-project JSON bundle import wizard.

## Troubleshooting

- **`401 Unauthorized` (with `WWW-Authenticate` header)**: Missing, expired, or revoked `Authorization: Bearer <token>`, or an uncompleted OAuth flow. Check the Bearer token or re-authorize the OAuth client.
- **`403` (`auth.token.scope_insufficient`)**: Even if your user role permits the action, the current PAT or OAuth grant lacks a required Scope. Check `requiredScopes` with `get_tool_details` and issue or authorize a credential that includes it.
- **`ai.tool_preview_lock_required`**: An `L2` write tool was passed to `execute_tool` without first calling `preview_tool`, or `arguments` changed after `previewLock` was issued. Call `preview_tool` with the exact arguments, confirm with the user, and pass `previewLock` unchanged.
- **`ai.preview_conflict`**: The target resource changed between `preview_tool` and `execute_tool`. Re-run `preview_tool` to inspect the latest state before retrying.
