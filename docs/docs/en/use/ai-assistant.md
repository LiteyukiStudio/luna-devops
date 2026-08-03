# AI Assistant

The AI assistant uses the current page context and your platform permissions to query resources, explain status, and help with routine operations.

On mobile, the assistant fills the currently visible Safari or browser viewport and keeps the composer available when the keyboard opens. Collapsed tool calls show only their execution status; expand a call to inspect its duration, arguments, result, and identifiers. The conversation browser is a dedicated mobile view that temporarily hides the composer and suggested actions. You can switch between conversations without closing the list, then use Back to return to chat.

## Get started

A platform administrator first configures these values under **Global Settings → AI Assistant**:

- Provider URL
- API key
- Model name

The Provider must expose an OpenAI-compatible `chat/completions` endpoint with SSE streaming and function tool calling.

After saving and enabling it, the assistant entry appears in the lower-right corner of the console.

Tap the floating entry on desktop or mobile to open the assistant. Drag it to change its position; small finger movement during a tap is not treated as a drag. On desktop, a compact assistant opens conversations as an overlay drawer so the chat is not squeezed. Widening the assistant automatically changes the drawer into a side-by-side sidebar. Switching conversations keeps the browser open; press Escape, click the scrim, or use Back to leave the overlay drawer.

## Conversations

In the assistant you can:

- Describe what you want to inspect or change
- Create, switch, rename, and batch-delete conversations
- Switch conversations while another response is running
- Stop a response at any time

An empty new conversation is not saved repeatedly. Conversations are named automatically until you rename one manually; a manually chosen title is never overwritten.

Closing and reopening the assistant returns to the conversation you were using. A full browser refresh starts with a new empty conversation instead. For eight seconds, a notice at the top of the message area lets you return to the previous conversation when needed. The conversation list stays open while switching conversations so you can continue browsing or managing them.

When an interactive form selects a project space, application, cluster, or another platform resource, the selector shows only its readable name. The submitted conversation message includes both the resource name and ID so you can verify the target and continue the operation unambiguously.

Each message shows its timestamp underneath. Hover anywhere over the message, including the timestamp area, to copy it; your own messages can also be sent again unchanged. These actions remain visible on touch devices.

## Queries and operations

The assistant has the same platform permissions as you. It cannot read or change a project space or resource that your account cannot access.

A request may require several resource queries. The assistant continues from each tool result and explains or corrects failed queries instead of treating an unfinished step as the final response.

A task has no fixed platform tool-call limit, so the assistant can keep querying, acting, and reading back results as the workflow requires. Run timeouts and model-iteration safeguards remain in place to stop abnormal infinite execution; when one is reached, completed operation records are preserved and the task is reported as unfinished.

Text, reasoning status, and tool calls appear in their actual workflow order while a response streams. Reconnecting, switching conversations, or refreshing preserves that same order instead of moving all tools to the top of the reply.

The assistant uses the same platform business APIs and the current signed-in user permissions as the web console; it does not maintain a lower-privilege or behaviorally different API subset. Protocol endpoints such as login callbacks, webhooks, terminal streams, and raw file downloads are not exposed directly to the model. The assistant explains any user step required for those interactions.

The assistant follows complete workflows for common tasks, including delivering applications from marketplace templates, existing images, or source repositories; configuring builds, releases, runtime, and gateways; diagnosing build or application failures; and handling project members, service relationships, notifications, security, and billing. Each workflow discovers trusted resources, collects required input, runs available operations, and reads back the result for acceptance. If the platform has not exposed the required operation, the assistant clearly identifies the blocked stage.

The assistant can also search the public web and read HTTP/HTTPS pages. Paste a GitHub repository, project website, or deployment guide to extract build methods, start commands, ports, and environment-variable names, then generate a deployment form with supported defaults. You still choose or confirm the target project space, domain, resource sizing, and secrets.

You only need to describe the outcome; you do not need to write an expert procedural prompt. When you provide a GitHub repository and ask for deployment, the assistant checks the README, actual directory tree, Dockerfiles, dependency manifests, configuration examples, and migrations. It reports conflicts between documentation and the current code and uses executable facts from the selected revision to build a reviewable configuration. Multi-service repositories are split along independent build and release boundaries instead of being forced into one application.

Web requests do not carry your browser cookies, tokens, or Git credentials. URLs containing embedded credentials, tokens, or signature parameters are rejected, and external content is treated as untrusted data. Connect a Git Provider in Luna DevOps before accessing a private repository.

If the server needs a proxy for public web access, a platform administrator can enable a dedicated pool under **Global Settings → AI Assistant → Web-tool proxy pool**. Enter one `http://user:password@host:port` or HTTPS proxy URL per line. Web search and page fetching rotate through the pool without affecting the model API, Git, builds, or other platform requests. Proxy credentials are encrypted and never displayed.

Read-only queries run directly. High-risk operations such as deletion, release, or permission changes show a confirmation card first. You can:

- Approve this operation
- Reject it
- Approve all pending operations already shown in the current task

Approval is bound to the complete arguments shown in the card; changed arguments require a new approval. Approval never expands your platform permissions, and the backend still performs the final authorization check.

## Suggestions and navigation

The row above the composer contains quick suggestions. Before the first message in a new conversation, the platform shows three to five common actions for the current page. After the conversation starts, the row switches to the next steps generated for the completed assistant response. A suggestion may:

- Continue the conversation
- Open a related platform page
- Run one clearly described follow-up operation

Suggestions are single-line pills floating over the bottom of the message area instead of occupying a divided toolbar. The message list keeps enough bottom scroll space so the final message can always move fully above the overlay. Long labels are truncated. Primary, standard, and high-risk actions use distinct semantic styles. New suggestions enter from the right with a spring transition, while the system’s reduced-motion preference removes the displacement.

If you are new to the platform, select one of the page suggestions in a new conversation, or ask “What can you do?” or “How should I get started?”. The assistant uses the current page and your permissions to offer two to five selectable goals, then continues the selected workflow.

For reading tasks, the assistant may include internal links. When it needs a decision, it prefers selectable options instead of moving you away from the current page.
After a conversation starts, the platform hides the row when the model does not return reliable contextual suggestions instead of restoring unrelated new-conversation presets.

When you explicitly ask to open a page, or your intent clearly belongs to another page and switching would make inspection, configuration, diagnosis, or review more coherent, the assistant can proactively switch the current tab without a reload. For example, it can open Billing when you ask about usage from the Dashboard, or move to an application's Builds tab after resolving that application. Navigation only synchronizes the visible context; it never replaces queries, platform operations, or acceptance checks.

Each assistant-driven route change appears in its actual timeline position as a compact navigation badge rather than a regular tool card. The badge names the destination and can be clicked to open it again. An unfinished navigation is retried briefly after a temporary disconnect, and the platform confirms success only after the target page has opened. The assistant does not jump ahead while multiple targets remain possible, a trusted resource ID is missing, or a form or approval is waiting.

## Interaction cards

The assistant can organize resource candidates, comparisons, resource details, configuration wizards, diagnostics, execution plans, task progress, receipts, and health summaries into cards. Continue or action buttons stay disabled until required fields are valid.

Cards are either presentational or interactive. Presentational cards only show facts or results already obtained. When the current task needs you to choose, enter, or confirm something, the assistant shows actionable candidate buttons, a selection field, or a form with an explicit submit action instead of asking you to answer from a display-only list.

Common workflows use platform presets for candidate selection, resource configuration, change review, diagnosis reports, execution progress, operation results, and health overviews. These presets keep fields, states, and actions consistent. The assistant only composes a more general card when a preset cannot express the required content.

A card is only an input or presentation step in a workflow; it does not mean that the platform operation is complete. For creation, installation, release, or repair tasks, the assistant continues with the corresponding operation and reads back the actual state. It reports accepted work as “submitted” or “in progress” and only reports completion after the target state is reached.

Deployment-target operations use the same complete parameter contract as the console, including source type, repository binding or image, runtime cluster, ports, resource sizing, build settings, runtime configuration, data volumes, and advanced Kubernetes settings. Repository sources must bind a repository from the current application, while image sources must provide an image reference. The platform validates field types, ranges, and resource ownership before execution and does not create a partial deployment target from incomplete arguments.

When creation, installation, or configuration needs a name, identifier, port, domain, toggle, or resource selection, the assistant presents a form or step-by-step wizard instead of asking you to copy and complete a message template.

When a form continues the conversation, only the non-sensitive fields declared by the card are included. Passwords, tokens, and secrets are never added to the message history. Any resulting operation still goes through platform authorization and required confirmation.

Complex cards show a preparation animation and replace it in place when the final content is ready. Cards adapt to the assistant window width, while wide tables and code scroll inside the card. Descriptive text supports safe Markdown; HTML and scripts are never executed as interface content.

## Page context and privacy

The assistant receives structured context such as the current route, page type, and selected resource IDs. Passwords, tokens, and secrets shown or entered on a page are not included in that context.

Tool details are collapsed by default. The collapsed row uses a localized business name and shows a finished call as “status · duration.” Expand it to inspect three sections: identifiers, arguments, and return value. Identifiers include the original operation, call ID, run ID, Trace ID, duration, and request ID when available. Redacted and bounded nested arguments and response data retain their JSON structure for direct inspection. Tokens, secrets, passwords, and authentication data are never displayed.

If interaction-card generation fails, the tool details list the invalid field paths and reasons so the card can be regenerated or an administrator can investigate with the request ID.

## Troubleshooting

### The assistant is missing

Ask a platform administrator to verify that the assistant is enabled and check the Provider URL, API key, and model name.

### The model is unavailable

Check Provider quota, model name, and API key, then retry.

### A tool failed

Expand the tool details to inspect the error code, response data, validation details, and request ID. Common causes include a resource changing, missing permission, an unavailable dependency, invalid arguments, or an operation that needs confirmation again.

`ai.approval_arguments_changed` means the execution arguments did not match the confirmation card. The platform blocked the request before the operation started. Generate a new confirmation card and review its arguments.

### A web page cannot be read

A platform administrator can manage domain and IP blocklists under **Global Settings → Security**. The assistant stops and reports a tool error when a target matches a blocklist, the response is not readable text, the response is too large, there are too many redirects, or the remote server returns an error.

When the dedicated proxy pool is enabled, verify that at least one proxy is reachable and its credentials are valid. Disable the pool to return to direct access; the saved proxy URLs remain available for later use.
