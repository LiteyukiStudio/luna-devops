# Companion Skills

Luna DevOps maintains a single unified source of workflow instructions (`skills/`) so external AI agents follow progressive single-domain tool search, schema disambiguation, mandatory `L2` preview-lock steps, and Web Console boundaries. Complete [MCP Integration](./index) first if you have not connected `/api/v1/mcp` yet.

You can use Skills in two ways.

## Option 1: Online via MCP (zero local install, recommended)

Any client connected to `/api/v1/mcp` can load workflow instructions that automatically match the running server version without copying files locally:

- **Call the meta-tool**: Invoke `load_skill` with 1–3 relevant `skillId` values for the current task.
- **Read MCP Resources / Prompts**: Read the entry resource `luna://skills/luna-devops`, domain resources `luna://skills/domains/<skillId>`, or invoke the corresponding MCP Prompts exposed by the server.

### 7 domain workflows (`skillId`)

| `skillId` | Scope and typical tasks |
| --- | --- |
| `workspace-iam` | Project spaces, boards, member roles, network isolation policies, cross-project service sharing, personal security summary, and credential revocation. |
| `source-registry-build` | Code repository discovery and binding, image registries and push/pull credentials, build variables, build configs, and build runs. |
| `application-deployment` | Application CRUD, App Marketplace template installation, shared runtime configs, deploy config specs/env/secrets/volume mounts, scaling, releases, and rollbacks. |
| `runtime-volumes` | Runtime cluster status, one-stop deploy config diagnostics, Pods/events/logs, single-shot container commands, persistent volumes (PVCs), and deployment Hooks. |
| `gateway-topology` | Access routes (domains and TLS certificate status), domain conflict and DNS checks, project topology, and service bindings. |
| `diagnostics-governance` | Cross-domain troubleshooting order, global search and event stream, personal billing and inbox, and personal/project notification rules. |
| `platform-admin` | Platform administrator settings, AI models and telemetry, users/OIDC/mail policies, App Marketplace templates, billing rate adjustments, and data retention. |

## Option 2: Local `SKILL.md` installation (optional)

If your agent supports local `SKILL.md` directories (such as Claude Code, Cursor, Codex, or Antigravity), copy both `skills/luna-devops` and `skills/domains` from [`LiteyukiStudio/luna-devops`](https://github.com/LiteyukiStudio/luna-devops/tree/main/skills) into your agent's skills folder so the agent knows the progressive routing rules upfront before its first MCP call.

For example, to install into Claude Code's user-level `~/.claude/skills` directory (substitute `.cursor/skills`, `~/.codex/skills`, or workspace `.claude/skills` for other clients):

```bash
git clone --depth 1 https://github.com/LiteyukiStudio/luna-devops.git /tmp/luna-devops
mkdir -p ~/.claude/skills
cp -R /tmp/luna-devops/skills/luna-devops /tmp/luna-devops/skills/domains ~/.claude/skills/
rm -rf /tmp/luna-devops
```

Keep `luna-devops/` and `domains/` side by side so relative references (`../domains/<skillId>/SKILL.md`) in `luna-devops/SKILL.md` resolve cleanly. After upgrading your Luna DevOps instance, re-sync this folder or rely on `load_skill` to read the server's bundled version.
