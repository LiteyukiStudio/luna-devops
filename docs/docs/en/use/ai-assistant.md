# AI Assistant

AI Assistant uses the current page and your permissions to query resources, explain status, analyze logs, and help with deployments and daily operations.

## What it can do

- Find project spaces, applications, builds, Releases, clusters, and routes.
- Analyze failed states and provide verifiable troubleshooting steps.
- Prepare deployments from a repository, an existing image, or an App Marketplace template.
- Collect parameters in forms and execute platform operations after confirmation.

Describe the goal directly, such as “check why this application is unreachable” or “deploy this API repository.” If containers or Kubernetes are new to you, say that this is your first deployment and the assistant will explain only the required concepts.

## Boundaries

> AI Assistant is an operational and diagnostic interface. It does not bypass authorization or replace human release verification.

- It can access only resources that you can access.
- Deletion, release, permission changes, and other high-risk operations show targets and parameters before confirmation. Changed parameters require a new confirmation.
- Builds, Releases, and other asynchronous work are complete only after the authoritative status reaches a terminal state.
- If an external service is unavailable, permission is missing, or the platform lacks a capability, the assistant reports the unfinished step instead of treating an accepted request as success.
- AI can misunderstand a request. Review the resource, environment, image version, and impact before approving a change.

## Information security

- Page context excludes plaintext passwords, tokens, and Secrets.
- Secret values go through controlled forms and tools directly to the platform, not ordinary chat messages, and are not displayed after saving.
- Public web reads never carry browser cookies, sessions, tokens, or Git credentials. Connect a Git Provider before using a private repository.
- Do not paste private keys, production database passwords, complete kubeconfigs, or other long-lived credentials into chat.
- Tool details and diagnostics may contain resource names, logs, and configuration fragments. Handle them as internal operations data.

## Start using it

An administrator must configure an OpenAI `chat/completions`-compatible Provider, enabled models, access scope, and personal-wallet billing under **Global Settings → AI Assistant**. Once enabled, the entry appears in the lower-right corner.

If the entry is missing, ask an administrator to check enablement and account scope. If the model is temporarily unavailable, keep the conversation and retry later.

The context ratio beside the composer uses only the current model's latest official `prompt_tokens` and the model-window snapshot captured for that call. A new conversation with no model call yet starts at 0%. If the Provider did not return verifiable usage or the model has changed, the UI shows only a gray ring; hover or keyboard focus reveals **No Provider usage data**. It never estimates or reuses an old percentage.
