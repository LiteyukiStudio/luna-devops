# AI Assistant

The AI assistant uses the current page and your platform permissions to inspect resources, explain status, diagnose failures, and help with deployments and routine operations.

When a containerized application supports both a network database and a local file database such as SQLite, the assistant first evaluates compatible databases already available in the project space, then considers deploying a new network database. It recommends local file storage only when those paths are unsuitable. An explicit database choice from you always takes precedence.

While a reply is being generated, the message list follows new content only when you are already at the latest message. Scrolling up pauses following until you return to the bottom.

Stopping an active task preserves the conversation content already produced and marks the turn as canceled. A normal cancellation is not reported as a model-generation failure. You can then edit the input and start another task.

If you're new to servers, containers, or Kubernetes, just say so ("I'm new to this" or "first time deploying"). The assistant switches to a guided mode that explains each concept in plain terms, first confirming your project type, start command, and port, then helping you package, deploy, and expose your app.

## Get started

An administrator enters the provider URL and API key under **Global Settings → AI Assistant**, then adds each model's name, maximum context tokens, maximum output tokens, and four prices (input, output, cached input, and cached output) in the **AI model catalog**. Use conservative limits published by the provider; the output limit must be lower than the context limit. The catalog always keeps at least one model enabled: its first model is enabled automatically, and its final enabled model cannot be disabled. All signed-in users can use it by default, or the administrator can restrict access to platform administrators. The provider must support an OpenAI-compatible `chat/completions` API, streaming, and tool calls.

The model picker in the lower-left corner of the composer selects an enabled model for the current conversation. The preference is conversation-scoped: a new conversation records its initial model and keeps using it until you explicitly switch that conversation, without affecting any other conversation. The selection is locked while a Run is active. Each Run stores the model identity, capabilities, all four prices, and cumulative budgets as an immutable snapshot, so disabling a model or changing its configuration does not alter an existing Run. If no model is available, an administrator must add and enable one first.

The assistant retries transient network errors, timeouts, rate limits, and server failures five times by default with exponential backoff. Administrators can set 0–10 retries under **Advanced runtime settings**; 0 disables retries. A stream is never replayed after visible output has started, and non-idempotent writes are not resubmitted when the outcome is unknown, preventing duplicate content or resources.

Some OpenAI-compatible providers in thinking mode require the prior assistant reasoning field when a tool call resumes. The platform adds that compatibility field and retries once only when the provider explicitly reports this parameter error and no visible output has started. The tool itself is not executed again, and unrelated parameter errors do not trigger this compatibility retry.

After it is enabled, open the assistant from the lower-right corner. You can ask it to:

- Find out why an application is unreachable.
- Prepare deployment settings from a source repository.
- Inspect recent failed builds and suggest a fix.
- Create, switch, or manage conversations.

On desktop, you can move and resize the assistant window. In a compact window, the conversation browser opens as an overlay drawer. When the window is wide enough, it opens automatically as a left sidebar and can still be collapsed from the conversation button in the header.

The conversation browser shortens update timestamps by age: conversations updated today show only the time, earlier entries from the current year show month, day, and time, and older entries include the year. A running conversation shows its activity indicator beside the timestamp without taking space from the title.

When you open a long conversation, the assistant first shows the most recent complete turns. Scrolling near the top loads older content while preserving your reading position, avoiding the delay of loading the entire history at once.

Long conversations retain their complete history for review. Before a request approaches the model context limit, earlier turns are summarized into task goals, constraints, resources, and pending work. Each model request prioritizes the current work, recent verbatim turns, and those retained facts, so a long-running task does not require a new conversation merely because its turn count grows.

Administrators can change the **Context input budget** under advanced runtime settings. It ranges from 64K to 2048K tokens and defaults to 1024K, providing more room for log analysis and complex DevOps diagnostics. The maximum output per reply defaults to 64K tokens and can be configured up to 128K when the model supports it. These are per-request ceilings rather than fixed usage; the effective input budget is automatically tightened by the selected model's context capacity and reserved output.

The same page sets cumulative limits for one Run: 2,000,000 tokens and 10,000 credits by default. The credit limit covers normal input, output, cached input, and cached output, while the user's available personal-wallet balance remains the smaller limit. Main replies, tool loops, summaries, titles, and next-step predictions all count toward the same Run. Before each request, the platform automatically tightens the output allowance. If the context window, Run budget, or wallet can no longer cover the request, the model request is not sent.

Each Run also has a high tool-call safety guard. It defaults to 256 platform-tool calls and can be configured from 32 to 2048. This guard stops repeated deterministic failures and queries that return no new information; it is not a target for normal workflows. When reached, the assistant reports why it stopped instead of claiming success. Legitimate asynchronous work may continue polling according to its authoritative task state.

The single ring at the bottom of the composer shows the context usage reported by the latest main assistant model call. Hover or focus the ring to see both that context usage and the current Run's cumulative token-budget usage, including each limit and percentage. Reopening a conversation restores the latest usage from the authoritative timeline.

Under **Advanced settings**, the Agent's internal parameters can be tuned by category: context & compression (compression trigger/target ratios, recent-turn retention, summary budgets, etc.), model & execution (max output tokens per reply, max model steps per Run, user-input size limit, navigation-action TTL), and tool results & cards (tool-result context budget, interaction-card repair limit). Every item ships with a platform default and is delivered to the Agent dynamically; keep the defaults for ordinary deployments and avoid tuning without a specific need.

After an upgrade, an existing runtime value may fall outside the current range or use an obsolete format. The platform preserves that stored value and does not block Agent startup; runtime execution uses the current platform default for that item instead. The next time an administrator submits that setting, the value must satisfy the range shown in the settings page. Provider endpoints, model capability catalogs, and tool contracts are not guessed or silently repaired. Incomplete structures still fail closed to avoid contacting the wrong target or relying on fabricated model capacity.

To discard runtime tuning, select **Restore defaults** beside **Save settings**. This restores only Agent runtime, context, model-execution, and tool-budget settings; provider details, API keys, access scope, observability, and proxy settings stay unchanged. Review the restored values, then select **Save settings** to apply them.

## Capabilities and operations

The assistant can inspect platform resources, analyze logs and status, read public web pages, collect settings through interactive cards, and report operation results. Long-running builds and releases show their real status, and the assistant checks the outcome after they finish.

An interaction card first shows a preparation placeholder at its actual conversation position, then replaces that same item with validated content. When a reply contains exactly one blocking configuration form, the assistant may place it at the end of that reply so you can read the explanation before filling it in. Candidate lists, multiple cards, progress, and results stay in their real event order.

The assistant uses a narrow, controlled card for each intent: resource choice, additional input, change review, diagnosis, health overview, execution progress, and operation result. Small candidate sets are shown directly; single-choice sets with six or more options include search. Selection messages retain the readable name and stable ID, while tool arguments continue to use the raw ID.

Password and secret fields in configuration forms can be entered directly and are displayed as password inputs. The assistant never prefills these fields; only a non-empty value that you enter for the current action is submitted, while an empty field means no change. When you submit a tool action, the secret travels only through the controlled tool execution path; it is not added to a normal chat message or model context, and message actions cannot reference it.

Runtime secrets for a deployment target must be submitted through the security form. User-entered `values` travel only through a Direct Tool Action; generated credentials are created and stored by the platform backend, so neither the assistant nor the browser receives the generated plaintext. Clearing requires a separate explicit clear action—an empty input never clears an existing value. Users explicitly choose ordinary environment variables as `public`; the platform no longer infers sensitivity from names, URLs, or values and does not block them. Choosing an ordinary variable means the value does not receive secret encryption or non-disclosure guarantees. Approval summaries and results report only which secret fields will be set, generated, or cleared and their status; they never return plaintext secrets.

After a card submits a platform operation, a new turn immediately shows the real waiting-for-approval, waiting-for-MFA, succeeded, or structured-failure state. Array arguments such as secret-item lists are assembled at their declared card paths. The platform still validates the complete tool schema and returns invalid field paths and allowed values instead of reducing every validation failure to a generic missing-input prompt.

Next-step options generated for a reply appear inside that assistant bubble at their real position among messages, reasoning, and tool calls, making their context clear. Before the first message in a new conversation, page presets remain above the composer for quick access to common tasks.

When a high-risk tool is waiting for approval or MFA step-up, its action card appears directly below the tool row without requiring you to expand the details. Parameters, results, and invocation identifiers remain collapsed until you choose to inspect them.

After MFA succeeds, the platform issues a short-lived step-up assertion bound to the current user, authenticated session, and operation purpose, then hands it to the assistant to resume the blocked tool call. TOTP codes are replay-protected and recovery codes are single-use, so you should not need to enter the same credential again for that tool call. If verification succeeds but the tool does not continue, refresh the conversation to confirm its current state instead of repeatedly submitting the same code.

Cards use five stable roles: candidate discovery and comparison (`candidates`), structured configuration (`form`), change review (`change_review`), facts or operation outcomes (`result`), and platform-backed live tasks (`live_task`). Resource details, diagnostics, charts, and tables remain available as content blocks; you do not need to select or know these template names.

A conversation can contain several cards with the same configuration fields. Each card keeps its inputs and selections isolated, so operating the current card does not change or jump to an earlier card. If one generated element cannot be displayed, only that position shows a fallback notice; the rest of the conversation and its cards remain available, and refreshing can restore them from the saved conversation.

When the platform has many tools, the assistant only uses capabilities that have been audited and explicitly admitted. The system evaluates relevant tools for the current goal and searches the catalog within the current turn when the current set is insufficient. Tool discovery does not bypass your permissions or mean that a business operation has already run, and you do not need to know tool names. Administrators can inspect each search and its result in Agent observability tool details.

When creating asynchronous resources such as releases, builds, or gateway routes, the assistant extracts the real resource ID from the platform's business result and then invokes the contract-declared detail tool to read back authoritative state. An accepted HTTP request does not mean that the task has succeeded. If the same recent read has already produced an empty result, the assistant reports that no resource was found instead of repeating the request across adjacent turns; an explicit request to refresh live state still performs a new observation.

When deploying from a repository or official deployment guide, the assistant first evaluates official container images maintained by the project. It normally recommends direct image deployment when the version, architecture, and pull requirements match the target. It falls back to a source build when the image cannot be verified, does not fit the deployment, or you explicitly request a source build.

When a project provides both Kubernetes/Helm and Docker/Compose deployment material, the assistant prefers Kubernetes/Helm configuration that matches the target version and uses Docker/Compose to fill in service topology and runtime parameters. If the Kubernetes material is outdated, incomplete, or unsuitable for the target environment, it uses verifiable Docker/Compose or source configuration instead.

The assistant has the same platform permissions as you. It cannot access projects or resources that you cannot access. Deletions, releases, permission changes, and other high-risk operations show their targets and parameters before execution. Changed parameters require a new approval.

When discovering project spaces, the assistant searches only project spaces directly related to you by default. Even for a platform administrator, it searches all project spaces only when you explicitly request a platform-wide search.

When diagnosis requires several container commands that share a working directory or environment, the assistant can open a short-lived runtime command session. Every command still revalidates permission, project policy, approval, and MFA. Idle sessions and sessions that reach their maximum lifetime close automatically, and the assistant releases them when diagnosis ends. Do not use command sessions for long-running jobs or secret storage.

If an operation requires an unavailable platform capability, additional permission, or manual work, the assistant identifies the unfinished step.

## Page context and privacy

The assistant receives the current page and selected resource context needed to understand your request. Passwords, tokens, and secrets are not sent as page context or stored in interactive-card messages.

External pages do not receive your login cookies, tokens, or Git credentials. Connect private repositories through a Git provider first.

Tool details show redacted parameters, results, and troubleshooting identifiers. Internal tools remain hidden unless an administrator enables debug mode to investigate assistant behavior.

## FAQ

### The assistant is missing

Ask an administrator to confirm that the assistant is enabled and that its access scope includes your account. The entry point depends only on platform enablement and access policy. It remains visible when the Agent or model service is temporarily unreachable, and a retryable error is shown when you start an operation.

### The model is unavailable

Check provider availability, balance, the model name in the catalog, and the API key, then retry. Existing Runs keep their snapshot after a model is disabled; new messages can only select enabled models.

If the Agent startup log reports a missing `maxContextTokens` or `maxOutputTokens` value, confirm that Luna API and Agent come from the same release and upgrade them together. Luna API supplies model capabilities from the model catalog, so they do not need to be configured again in the Agent container.

### The context or Run budget is insufficient

For a context error, shorten the current input, remove unneeded history, or ask an administrator to verify the model capabilities. When a Run exhausts its cumulative token or credit budget, start a new task to continue. If the personal wallet is insufficient, recharge it or choose an affordable model. In each case, the platform stops before incurring a new model charge.

### A tool failed

Expand its details and inspect the error code and request ID. Common causes include changed resources, insufficient permission, unavailable dependencies, invalid parameters, or an operation that needs approval again.

For `ai.database_schema_mismatch`, start or restart Luna API first so it can apply database migrations automatically, then start the Agent. The Agent never changes the database schema itself. In managed mode, the first startup must obtain the authoritative provider and tool catalog from Luna API and exits if that fetch fails. After a successful startup, a transient configuration-refresh failure keeps using the last valid configuration.

For `ai.final_response_missing`, the model returned tools or options without a visible conclusion and did not repair the response within the bounded model steps. Retry the request, or verify that the Provider reliably returns message content.

### A web page cannot be read

The target may be blocked by security policy, may not contain readable text, or may be temporarily unavailable. If an outbound proxy pool is enabled, confirm that at least one proxy is reachable.
