---
name: luna-devops-navigation
description: Guide Luna DevOps assistant responses to link platform pages and resources with safe internal Markdown routes. Use when an answer references a page, project, application, build, deployment, gateway, billing, setting, or another resource that the user can open in the Luna DevOps web console.
---

# Luna DevOps internal navigation

## Output links

- Write an inline Markdown link when a referenced page or resource has a useful destination: `[label](/registered/path)`.
- Use a concise human label. Do not expose a raw path when a resource name is available.
- Use links in the answer itself for resources being discussed. End every normally completed turn with exactly one `create_options` call containing 2-5 context-specific next-step predictions.
- Use only IDs returned by trusted tool results or supplied page context. Never invent an ID or infer one from a display name.
- Do not put internal links in code fences and do not claim that emitting a link navigated the browser.

## Registered pages

| Page | Path |
| --- | --- |
| Dashboard | `/dashboard` |
| Projects | `/projects` |
| Events | `/events` |
| Code repositories | `/code-repositories` |
| Registries | `/registries` |
| Clusters | `/clusters` |
| Application marketplace | `/app-templates` |
| Billing | `/billing` |
| Account | `/settings/account` |
| Authentication providers | `/settings/auth-providers` |
| Notifications | `/settings/notifications` |
| Operations | `/settings/operations` |
| Site settings | `/settings/site` |
| Users | `/settings/users` |

## Resource paths

- Project: `/projects/:projectId`
- Project tab: `/projects/:projectId?tab=:tabId`
- Application: `/projects/:projectId/apps/:applicationId`
- Application tab: `/projects/:projectId/apps/:applicationId?tab=:tabId`
- Build run: `/projects/:projectId/apps/:applicationId?tab=builds#tab=builds&buildRunId=:buildRunId`

Project tab IDs: `overview`, `apps`, `members`, `build-variables`, `runtime-configs`, `hooks`, `topology`.

Application tab IDs: `overview`, `repositories`, `builds`, `deployments`, `gateway`, `topology`, `settings`.

## Safety

- Use a root-relative path beginning with exactly one `/`.
- Never emit `javascript:`, `data:`, protocol-relative, external, or unregistered URLs as internal links.
- Preserve authorization boundaries. A link is navigation only and does not grant access or execute an operation.
- If the destination or required IDs are uncertain, describe where to go without fabricating a link.
