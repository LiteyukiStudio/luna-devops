# Settings and Connections

This page explains how the platform connects to Git, registries, and runtime clusters, and which settings affect security or the console. Settings broadly fall into two groups: public site information and backend-only connections to external systems.

## Public settings

Public settings control what users see:

- Site title.
- Logo and favicon.
- Sign-in subtitle.
- Theme and language preferences.

These can be shown to the frontend, but should never contain tokens, passwords, or internal-only addresses.

## Security policy

In production mode, API responses for the console include security headers such as `Content-Security-Policy`, `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, and `Permissions-Policy`. The default CSP allows only same-origin scripts, manifests, and connections, blocks plugin objects, and permits inline styles needed by Tailwind/shadcn, `data:` fonts and images, and HTTPS images.

`Strict-Transport-Security` should only be enabled for production HTTPS deployments. It is enabled by default when `APP_ENV=production`; use `APP_ENABLE_HSTS=true` to force it on, or `APP_ENABLE_HSTS=false` to disable it. Do not enable HSTS for local or test domains that still need HTTP access.

Step-up verification is controlled by `security.stepUpMfa.enabled` and is disabled by default. Security checks read this policy from shared PostgreSQL state, so every API replica uses the same value. A database read failure fails closed as enabled MFA with shorter timeouts instead of bypassing verification. Before enabling it, at least one available platform administrator must enroll an offline TOTP authenticator from Account Security. While the global policy is enabled, the last MFA-enabled platform administrator cannot disable MFA, be disabled, or be demoted. Policy updates, MFA disablement or reset, and administrator-account state changes share a PostgreSQL transaction lock. While holding it, the current transaction rereads the policy and revalidates the actor, session, Step-up assertion, and available administrators. This stops stale requests that were waiting for the lock and prevents concurrent requests from leaving the policy enabled with nobody able to verify. `security.stepUpMfa.idleTimeoutMinutes` controls how long an assertion remains active without another sensitive operation and defaults to 10 minutes. `security.stepUpMfa.absoluteTimeoutMinutes` is the hard lifetime even while activity continues and defaults to 60 minutes.

The site-settings form submits only values that actually changed. The backend also compares the current policy and requests `security_settings_update` verification only when a `security.stepUpMfa.*` value really changes. Updating branding, the operations dashboard, or other ordinary settings is not treated as a security-policy change merely because an unchanged security field appeared in a request.

Enrollment requires primary reauthentication first: local accounts enter the current password, while OIDC accounts must have completed primary authentication within the last five minutes and cannot use an impersonated session. Remember-token recovery creates a new session but never refreshes this primary-authentication time. The page then shows a QR code, the complete `otpauth` URI, and the manual secret. MFA is enabled only after a valid six-digit TOTP is confirmed. Verification accepts the current 30-second window and one adjacent window on either side, but the same or an older time-step code cannot be reused. Confirmation creates ten one-time recovery codes; plaintext is shown only once and the backend stores bcrypt hashes. Each recovery code can succeed once, and regeneration immediately invalidates every old code. The TOTP secret lives in the encrypted platform secret store, not as plaintext in a business table, and administrators cannot retrieve it.

When the global policy is enabled, Web Console, runtime commands, data export, secret and registry credential writes, kubeconfig updates, auth provider updates, platform-administrator account changes, and security-policy changes check a Step-up assertion for the current browser session and operation purpose. A missing assertion returns `mfa_required`; the console opens the shared authenticator/recovery-code dialog and retries the original request after verification. Assertions are shared in the database by user, session, and purpose. Successful operations refresh the idle deadline but never extend the absolute deadline. Personal access tokens cannot complete MFA or replace these interactive-session checks.

Enrollment, confirmation, and verification are rate-limited by user and source IP. Enrollment allows up to 10 consecutive attempts per hour, while confirmation and sensitive-operation verification allow up to 20 consecutive attempts per five minutes. A successful operation immediately clears that user's counter for the operation. Source IPs use a separate, higher threshold so users behind the same office or gateway NAT do not normally affect each other. Authenticator and recovery codes are never written to logs. Enrollment, disablement, recovery-code use and regeneration, policy updates, administrator resets, and successful or failed verification are audited. A platform administrator must complete `user_admin_update` Step-up verification before resetting another user's MFA. The endpoint cannot reset the current administrator and cannot remove the last available MFA-enabled administrator while the global policy is active. Password, role, or disabled-state changes revoke existing sessions, remember tokens, and Step-up assertions. Disabling or resetting MFA also deletes the TOTP secret, recovery codes, and current assertions.

## Git providers

Git providers connect GitHub or Gitea. After setup, users can bind repositories, receive webhooks, and trigger builds by branch or tag.

Deleting a Git provider also deletes all Git credentials that belong to it. Confirm that repository bindings and build flows no longer depend on those credentials before deleting.

If you only want to verify the deployment path, skip Git providers and start with an existing image. Connect the repository after the application runs successfully so early failures are easier to isolate.

## Registries

Registries store build output and provide the images pulled by runtime clusters. Common choices include Harbor, Gitea Registry, DockerHub, and generic OCI / Docker Registry.

Generic OCI registries use the standard Docker Registry HTTP API V2: the platform tests `/v2/`, searches repositories with `_catalog`, and reads tags with `tags/list`. Some registries disable catalog listing; in that case search may be unavailable, but users can still enter the repository path and tag manually.

Deleting a registry also deletes all credentials that belong to it. Confirm that deployment targets, build jobs, or runtime image pulls no longer depend on those credentials before deleting.

Automated builds need push credentials. Existing-image deployments mainly need the runtime cluster to pull the target image.

When creating a release, the platform first reads tags live from the target registry and repository stored on the deployment target. If the registry API is unavailable, credentials are insufficient, or the repository does not allow tag listing, the release dialog falls back to saved successful build records. Saved build records only prove that an image was built and pushed at that time; they do not guarantee the upstream registry still keeps that tag. If registry cleanup is enabled, confirm that images referenced by released versions are retained before releasing or rolling back.

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

When HTTPS should terminate on the in-cluster Gateway, administrators can configure the name and namespace of an existing Kubernetes TLS Secret on the runtime cluster. The platform writes that Secret as `tls.certificateRefs` on the shared Gateway HTTPS listener. The Secret content must be created by an administrator or an external certificate sync system first; manual mode does not store private keys in the platform or issue certificates automatically. Leaving the TLS Secret namespace empty references the Gateway namespace; cross-namespace references may also require a Gateway API `ReferenceGrant`.

When an access route uses the "HTTP Challenge certificate" mode, the platform creates a cert-manager `Certificate` from the runtime cluster's cert-manager settings and appends the resulting Secret to the shared Gateway HTTPS listener. Runtime clusters can configure the Issuer kind, Issuer name, and certificate namespace; when the Issuer name is empty, the Worker default `CERT_MANAGER_CLUSTER_ISSUER` is used. This phase creates the Certificate, reads its Ready state, and references the Secret. HTTP-01 reachability still depends on the cluster's cert-manager solver, Gateway HTTP listener, and public port 80 ingress.

When public port `80` is unavailable or a shared wildcard certificate is preferred, enable DNS-01 wildcard certificates on the runtime cluster. The platform creates a cert-manager `Certificate` containing the root domain and `*.root-domain`, then references the output Secret from the Gateway HTTPS listener. DNS API credentials, DNS-01 solver configuration, and ACME account management still belong to the selected Issuer / ClusterIssuer; the platform does not store DNS provider credentials.

If an outer Nginx/CDN/load balancer already owns host ports `80/443`, point it to the cluster Gateway's internal ports: upstream TLS termination should forward to the HTTP listener, such as `8080`; Gateway TLS termination should forward to the HTTPS listener, such as `8443`. The platform still renders access URLs from the runtime cluster's external access scheme and external access port settings.

The cluster resource page lists platform-managed namespaces, workloads, services, configs, secrets, and storage with server-side pagination. Only resources visible to the current user are counted in the page total. The workload tab uses Deployment rows as the top level; expanding a Deployment shows its Pods as child rows, and those Pod rows are not counted by pagination.

Platform administrators can open Web Console from Pod child rows in the workload tab. The frontend first calls the Pod terminal authorize preflight through the normal HTTP API. If it returns `mfa_required`, the shared MFA dialog verifies the `runtime_terminal` purpose and retries. A successful preflight only allows the connection attempt to continue. The WebSocket rechecks the session, platform-administrator role, MFA assertion, Pod identity, and platform ownership before upgrading and every three seconds while connected; losing any condition closes the shell. Only real terminal input refreshes the idle deadline, with writes throttled; resize, ping, and background polling do not keep it alive, and the absolute deadline never moves. Terminal activity is audited.

If the API or worker runs in a container, kubeconfig server addresses must be reachable from that container. Avoid host-only `127.0.0.1`.

Runtime clusters also host Kubernetes build Jobs. The small-team default allows 4 concurrent build Jobs per runtime cluster and 2 concurrent builds per project space. Extra builds stay queued and retry automatically instead of being marked failed immediately.

## Personal tokens

Personal tokens are used by scripts, CI, or external automation to call the platform API. The plain token is shown only once after creation, and the backend stores only a hash. Revoked tokens stop working immediately and are hidden from the list.

Tokens can include multiple scopes. The scope catalog is served by the backend and periodically synchronized by the frontend, so future scope changes do not require hardcoded page updates. Regular users can create read scopes and explicit automation trigger scopes, such as reading project spaces, reading deployments, triggering builds, and creating releases. Platform administrators can create higher-risk scopes such as write, delete, Web Console, secret value access, user management, and site configuration scopes.

Prefer least privilege. CI that only triggers builds should use `build:trigger`; automation that only creates releases should use `deployment:release`; log readers should add `build:read` or `deployment:read` only when needed. Avoid granting unnecessary write or management scopes to long-lived tokens.

## Secrets

Secrets, tokens, and registry credentials are not echoed back. When editing, an empty value means "keep the existing value". Enter a new value only when replacing it.
