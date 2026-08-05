# AI Assistant

The AI assistant uses the current page and your platform permissions to inspect resources, explain status, diagnose failures, and help with deployments and routine operations.

Stopping an active task preserves the conversation content already produced and marks the turn as canceled. A normal cancellation is not reported as a model-generation failure. You can then edit the input and start another task.

If you're new to servers, containers, or Kubernetes, just say so ("I'm new to this" or "first time deploying"). The assistant switches to a guided mode that explains each concept in plain terms, first confirming your project type, start command, and port, then helping you package, deploy, and expose your app.

## Get started

An administrator enters the provider URL, API key, and model under **Global Settings → AI Assistant**, then enables the assistant. All signed-in users can use it by default, or the administrator can restrict access to platform administrators. The provider must support an OpenAI-compatible `chat/completions` API, streaming, and tool calls.

After it is enabled, open the assistant from the lower-right corner. You can ask it to:

- Find out why an application is unreachable.
- Prepare deployment settings from a source repository.
- Inspect recent failed builds and suggest a fix.
- Create, switch, or manage conversations.

On desktop, you can move and resize the assistant window. In a compact window, the conversation browser opens as an overlay drawer. When the window is wide enough, it opens automatically as a left sidebar and can still be collapsed from the conversation button in the header.

The conversation browser shortens update timestamps by age: conversations updated today show only the time, earlier entries from the current year show month, day, and time, and older entries include the year. A running conversation shows its activity indicator beside the timestamp without taking space from the title.

Long conversations retain their complete history for review. Before a request approaches the model context limit, earlier turns are summarized into task goals, constraints, resources, and pending work. Each model request prioritizes the current work, recent verbatim turns, and those retained facts, so a long-running task does not require a new conversation merely because its turn count grows.

## Capabilities and operations

The assistant can inspect platform resources, analyze logs and status, read public web pages, collect settings through interactive cards, and report operation results. Long-running builds and releases show their real status, and the assistant checks the outcome after they finish.

When deploying from a repository or official deployment guide, the assistant first evaluates official container images maintained by the project. It normally recommends direct image deployment when the version, architecture, and pull requirements match the target. It falls back to a source build when the image cannot be verified, does not fit the deployment, or you explicitly request a source build.

When a project provides both Kubernetes/Helm and Docker/Compose deployment material, the assistant prefers Kubernetes/Helm configuration that matches the target version and uses Docker/Compose to fill in service topology and runtime parameters. If the Kubernetes material is outdated, incomplete, or unsuitable for the target environment, it uses verifiable Docker/Compose or source configuration instead.

The assistant has the same platform permissions as you. It cannot access projects or resources that you cannot access. Deletions, releases, permission changes, and other high-risk operations show their targets and parameters before execution. Changed parameters require a new approval.

When diagnosis requires several container commands that share a working directory or environment, the assistant can open a short-lived runtime command session. Every command still revalidates permission, project policy, approval, and MFA. Idle sessions and sessions that reach their maximum lifetime close automatically, and the assistant releases them when diagnosis ends. Do not use command sessions for long-running jobs or secret storage.

If an operation requires an unavailable platform capability, additional permission, or manual work, the assistant identifies the unfinished step.

## Page context and privacy

The assistant receives the current page and selected resource context needed to understand your request. Passwords, tokens, and secrets are not sent as page context or stored in interactive-card messages.

External pages do not receive your login cookies, tokens, or Git credentials. Connect private repositories through a Git provider first.

Tool details show redacted parameters, results, and troubleshooting identifiers. Internal tools remain hidden unless an administrator enables debug mode to investigate assistant behavior.

## FAQ

### The assistant is missing

Ask an administrator to confirm that it is enabled and that the provider URL, API key, and model are valid.

### The model is unavailable

Check provider availability, balance, model name, and API key, then retry.

### A tool failed

Expand its details and inspect the error code and request ID. Common causes include changed resources, insufficient permission, unavailable dependencies, invalid parameters, or an operation that needs approval again.

### A web page cannot be read

The target may be blocked by security policy, may not contain readable text, or may be temporarily unavailable. If an outbound proxy pool is enabled, confirm that at least one proxy is reachable.
