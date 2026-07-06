# Settings and Connections

Settings fall into two groups: public console settings and backend connections to external systems.

## Public settings

Public settings affect what users see:

- Site title.
- Logo and favicon.
- Sign-in subtitle.
- Theme and language preferences.

These can be shown to the frontend, but should never contain tokens, passwords, or internal-only addresses.

## Security policy

In production mode, API responses for the console include security headers such as `Content-Security-Policy`, `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, and `Permissions-Policy`. The default CSP allows only same-origin scripts and connections, inline styles needed by Tailwind/shadcn, `data:` fonts and images, and HTTPS images.

`Strict-Transport-Security` should only be enabled for production HTTPS deployments. It is enabled by default when `APP_ENV=production`; use `APP_ENABLE_HSTS=true` to force it on, or `APP_ENABLE_HSTS=false` to disable it. Do not enable HSTS for local or test domains that still need HTTP access.

Step-up verification for sensitive operations is controlled by the site config key `security.stepUpMfa.enabled` and is disabled by default. When enabled, Web Console, runtime terminal, data export, secret and registry credential writes, kubeconfig updates, auth provider updates, and administrator user changes require a matching Step-up assertion for the current session and purpose; otherwise the API returns the stable error code `mfa_required`. This version includes the backend checkpoint and shared assertion storage. TOTP enrollment, recovery codes, and the unified frontend MFA dialog are still tracked in TODO, so do not enable this policy in production until those interactions are complete.

## Git providers

Git providers connect GitHub or Gitea. After setup, users can bind repositories, receive webhooks, and trigger builds by branch or tag.

Deleting a Git provider also deletes all Git credentials that belong to it. Confirm that repository bindings and build flows no longer depend on those credentials before deleting.

If you only want to test deployment first, skip Git providers and use an existing image.

## Registries

Registries store or provide images. Common choices include Harbor, Gitea Registry, DockerHub, and generic OCI / Docker Registry.

Generic OCI registries use the standard Docker Registry HTTP API V2: the platform tests `/v2/`, searches repositories with `_catalog`, and reads tags with `tags/list`. Some registries disable catalog listing; in that case search may be unavailable, but users can still enter the repository path and tag manually.

Deleting a registry also deletes all credentials that belong to it. Confirm that deployment targets, build jobs, or runtime image pulls no longer depend on those credentials before deleting.

Automated builds need push credentials. Existing-image deployments mainly need the runtime cluster to pull the target image.

Deployment targets support Dockerfile Build Args. Users can enter one Dockerfile `ARG` per line as `KEY=value`; the platform snapshots the current config into the BuildRun and passes the values to BuildKit. Build Args support the same build-time templates as image tags: `${{ github.sha }}`, `${{ github.ref_name }}`, `${{ github.ref_type }}`, `${{ github.ref }}`, and `{short_sha}`. Build Args are build parameters and appear in build records, so do not use them for secrets. Put sensitive values in project-space build secret variables.

Registry credentials can define an image repository template and an image tag template. They are used only to seed the default push location when a deployment target is created. After the deployment target is saved, the repository and tag are stored as a snapshot and no longer follow credential template changes. For example, repository template `devopsns/{project}-{app}-{stage}` plus tag template `{projectSlug}-{appSlug}-{stage}` seeds image refs like `devopsns/blog-api-prod:blog-api-prod`.

Repository and tag templates both render only static values known when the deployment target is created: `{registryNamespace}`, `{project}`, `{projectSlug}`, `{app}`, `{appSlug}`, `{stage}`, and `{target}`. If the tag template uses build-time variables such as `{commit}` or `{branch}`, the deployment target default falls back to `latest` so future builds are not implicitly rewritten by credential templates.

## Runtime clusters

Runtime clusters are release targets. The platform turns Releases into Kubernetes resources, then shows status, logs, and diagnostics.

Runtime clusters also own available access-route domain suffixes, external access schemes, external access ports, and Gateway API defaults. A cluster can define multiple suffixes; when users create an access route, they choose one suffix from the deployment target's cluster. Access routes use the selected suffix to generate default domains, expand short host prefixes, and return console access links, so multiple clusters can use different GatewayClasses, shared Gateways, or root domains, and one cluster can expose public, internal, or business-specific domains.

Gateway config is split into external display and internal cluster layers:

- The external access scheme and external access port only affect generated access URLs. HTTP `80` and HTTPS `443` are omitted from URLs; non-standard ports are shown as `:port`. They do not change Kubernetes Gateway listeners or request certificates.
- Gateway listener names and ports are the internal Gateway/Controller settings that receive traffic inside the cluster. The defaults are `web:8080` and `websecure:8443`. Project users do not choose ports or listeners.

The external TLS mode decides which listener access routes bind to by default. "TLS terminates at Gateway" binds routes to the HTTPS listener, such as `websecure`; "TLS terminates upstream" binds routes to the HTTP listener, such as `web`, because traffic entering the Gateway is already cleartext HTTP. The HTTPS listener is always emitted as an HTTPS/TLS entrypoint to match Traefik `websecure` entryPoints with TLS enabled.

If an outer Nginx/CDN/load balancer already owns host ports `80/443`, point it to the cluster Gateway's internal ports: upstream TLS termination should forward to the HTTP listener, such as `8080`; Gateway TLS termination should forward to the HTTPS listener, such as `8443`. The platform still renders access URLs from the runtime cluster's external access scheme and external access port settings.

The cluster resource page lists platform-managed namespaces, workloads, services, configs, secrets, and storage with server-side pagination. Only resources visible to the current user are counted in the page total. The workload tab uses Deployment rows as the top level; expanding a Deployment shows its Pods as child rows, and those Pod rows are not counted by pagination.

Platform administrators can open Web Console from Pod child rows in the workload tab to enter an interactive terminal for that Pod. The entry reuses the application deployment Web Console terminal, verifies that the Pod is still a platform-managed resource, and writes audit logs. If sensitive-operation Step-up verification is enabled, the same `runtime_terminal` assertion is required before the console opens.

If the API or worker runs in a container, kubeconfig server addresses must be reachable from that container. Avoid host-only `127.0.0.1`.

Runtime clusters also host Kubernetes build Jobs. The small-team default allows 4 concurrent build Jobs per runtime cluster and 2 concurrent builds per project space. Extra builds stay queued and retry automatically instead of being marked failed immediately.

## Personal tokens

Personal tokens are used by scripts, CI, or external automation to call the platform API. The plain token is shown only once after creation, and the backend stores only a hash. Revoked tokens stop working immediately and are hidden from the list.

Tokens can include multiple scopes. The scope catalog is served by the backend and periodically synchronized by the frontend, so future scope changes do not require hardcoded page updates. Regular users can create read scopes and explicit automation trigger scopes, such as reading project spaces, reading deployments, triggering builds, and creating releases. Platform administrators can create higher-risk scopes such as write, delete, Web Console, secret value access, user management, and site configuration scopes.

Prefer least privilege. CI that only triggers builds should use `build:trigger`; automation that only creates releases should use `deployment:release`; log readers should add `build:read` or `deployment:read` only when needed. Avoid granting unnecessary write or management scopes to long-lived tokens.

## Secrets

Secrets, tokens, and registry credentials are not echoed back. When editing, an empty value means "keep the existing value". Enter a new value only when replacing it.
