# Settings and Connections

This page explains how the platform connects to Git, registries, and runtime clusters, and which settings affect security or the console. Settings broadly fall into two groups: public site information and backend-only connections to external systems.

## Public settings

Public settings control what users see:

- Site title.
- Logo and favicon.
- Sign-in subtitle.
- Theme and language preferences.

These can be shown to the frontend, but should never contain tokens, passwords, or internal-only addresses.

## Registration and email

Site Settings → Registration & Email exposes two independent registration controls. Email registration is off by default; when enabled, a new user must receive and submit a one-time email code. OIDC registration is on by default; disabling it blocks only first-time users with no bound identity and does not interrupt existing OIDC users.

Email registration requires the SMTP host, port, transport security, credentials, sender address, and sender name. The SMTP password is write-only: after it is saved, the UI reports only that a value exists. The actual value is stored in the Secret Store and never appears in ordinary configuration responses. Leave the password empty when editing other mail settings to preserve it.

“Allow passwordless accounts to set a local password” is off by default. When enabled, an account originally created through OIDC can add a password under Account → Security. Accounts that already have a password must verify the current password before changing it. Every successful password set or change revokes that account's existing sessions.

The global build environment has its own Build tab and is not part of Branding. Registration, mail, and account-enrollment policy live under Registration & Email, while MFA and sensitive-operation policy remain under Security.

### Brand color

Platform administrators can select the default brand color as the first setting under Site Settings → Branding. Users can continue following the platform or choose a personal color under Account → Profile. A personal selection takes priority. Following the platform stores an empty preference rather than a snapshot, so later administrator changes continue to reach those users. Brand color affects buttons, links within content, selected states, and focus rings only. Success, warning, and failure colors remain independent, as do light, dark, and system appearance modes.

Every option comes from the [official Radix Colors 3.0.0 brand scales](https://www.radix-ui.com/colors/docs/palette-composition/composing-a-palette): Gold, Bronze, Brown, Yellow, Amber, Orange, Tomato, Red, Ruby, Crimson, Pink, Plum, Purple, Violet, Iris, Indigo, Blue, Cyan, Teal, Jade, Green, Grass, Lime, Mint, and Sky. Arbitrary HEX values and custom CSS are not accepted. Yellow, Amber, Lime, Mint, and Sky use the dark foreground prescribed by Radix; the other solid scales use a white foreground.

Component backgrounds, hover states, borders, focus rings, solid controls, and text map to the corresponding Radix scale steps. The official CSS includes both sRGB and Display-P3 values and lets the browser select the supported gamut. In production, the API injects the site preset into the console HTML. Before React renders, the initialization script reads the active account's cached preference and applies the first available value in this order: personal preference, site default, then Blue. The authenticated user response subsequently reconciles that cache with the backend record, and signing out restores the site default, preventing a default-color flash during normal signed-in visits.

## Security policy

In production mode, API responses for the console include security headers such as `Content-Security-Policy`, `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, and `Permissions-Policy`. The default CSP allows only same-origin scripts, manifests, and connections, blocks plugin objects, and permits inline styles needed by Tailwind/shadcn, `data:` fonts and images, and HTTPS images.

`Strict-Transport-Security` should only be enabled for production HTTPS deployments. It is enabled by default when `APP_ENV=production`; use `APP_ENABLE_HSTS=true` to force it on, or `APP_ENABLE_HSTS=false` to disable it. Do not enable HSTS for local or test domains that still need HTTP access.

Step-up verification is controlled by `security.stepUpMfa.enabled` and is disabled by default. Security checks read this policy from shared PostgreSQL state, so every API replica uses the same value. A database read failure fails closed as enabled MFA with shorter timeouts instead of bypassing verification. Before enabling it, at least one available platform administrator must enroll an offline TOTP authenticator from Account Security. While the global policy is enabled, the last MFA-enabled platform administrator cannot disable MFA, be disabled, or be demoted. Policy updates, MFA disablement or reset, and administrator-account state changes share a PostgreSQL transaction lock. While holding it, the current transaction rereads the policy and revalidates the actor, session, Step-up assertion, and available administrators. This stops stale requests that were waiting for the lock and prevents concurrent requests from leaving the policy enabled with nobody able to verify. `security.stepUpMfa.idleTimeoutMinutes` controls how long an assertion remains active without another sensitive operation and defaults to 10 minutes. `security.stepUpMfa.absoluteTimeoutMinutes` is the hard lifetime even while activity continues and defaults to 60 minutes.

The site-settings form submits only values that actually changed. The backend also compares the current policy and requests `security_settings_update` verification only when a `security.stepUpMfa.*` value really changes. Updating branding, the operations dashboard, or other ordinary settings is not treated as a security-policy change merely because an unchanged security field appeared in a request.

Enrollment requires primary reauthentication first: local accounts enter the current password, while OIDC accounts must have completed primary authentication within the last five minutes and cannot use an impersonated session. Remember-token recovery creates a new session but never refreshes this primary-authentication time. The page then shows a QR code, the complete `otpauth` URI, and the manual secret. MFA is enabled only after a valid six-digit TOTP is confirmed. Verification accepts the current 30-second window and one adjacent window on either side, but the same or an older time-step code cannot be reused. Email registration and MFA share the same six-slot code input, with whole-code paste, a mobile numeric keypad, and one-time-code autofill from the operating system or password manager. Confirmation creates ten one-time recovery codes; plaintext is shown only once and the backend stores bcrypt hashes. Each recovery code can succeed once, and regeneration immediately invalidates every old code. The TOTP secret lives in the encrypted platform secret store, not as plaintext in a business table, and administrators cannot retrieve it.

When the global policy is enabled, Web Console, runtime commands, data export, secret and registry credential writes, kubeconfig updates, auth provider updates, platform-administrator account changes, and security-policy changes check a Step-up assertion for the current browser session and operation purpose. A missing assertion returns `mfa_required`; the console opens the shared authenticator/recovery-code dialog and retries the original request after verification. Assertions are shared in the database by user, session, and purpose. Successful operations refresh the idle deadline but never extend the absolute deadline. Personal access tokens cannot complete MFA or replace these interactive-session checks.

Enrollment, confirmation, and verification are rate-limited by user and source IP. Enrollment allows up to 10 consecutive attempts per hour, while confirmation and sensitive-operation verification allow up to 20 consecutive attempts per five minutes. A successful operation immediately clears that user's counter for the operation. Source IPs use a separate, higher threshold so users behind the same office or gateway NAT do not normally affect each other. Authenticator and recovery codes are never written to logs. Enrollment, disablement, recovery-code use and regeneration, policy updates, administrator resets, and successful or failed verification are audited. A platform administrator must complete `user_admin_update` Step-up verification before resetting another user's MFA. The endpoint cannot reset the current administrator and cannot remove the last available MFA-enabled administrator while the global policy is active. Password, role, or disabled-state changes revoke existing sessions, remember tokens, and Step-up assertions. Disabling or resetting MFA also deletes the TOTP secret, recovery codes, and current assertions.

## Git providers

Git providers connect GitHub or Gitea. After setup, users can bind repositories, receive webhooks, and trigger builds by branch or tag.

A Git credential's scope is its Luna DevOps usage boundary: personal credentials are available only to their creator, project-scoped credentials can be used by members of the selected project spaces in platform jobs, and global credentials are available to all projects. “Scopes” are the upstream API permissions granted by GitHub, Gitea, or GitLab, such as `repo` and `read:user`; they do not change ownership inside Luna DevOps. Apply least privilege to both layers when creating long-lived credentials.

You can edit a Git credential's scope and project bindings after creation, but it cannot exceed its Git Provider: a user provider only allows user credentials, a project provider allows user credentials or a subset of its projects, and only a global provider allows global credentials. Leaving access and refresh tokens empty while editing keeps their current values.

Deleting a Git provider also deletes all Git credentials that belong to it. Confirm that repository bindings and build flows no longer depend on those credentials before deleting.

If you only want to verify the deployment path, skip Git providers and start with an existing image. Connect the repository after the application runs successfully so early failures are easier to isolate.

## Registries

Registries store build output and provide the images pulled by runtime clusters. Common choices include Harbor, Gitea Registry, DockerHub, and generic OCI / Docker Registry.

Generic OCI registries use the standard Docker Registry HTTP API V2: the platform tests `/v2/`, searches repositories with `_catalog`, and reads tags with `tags/list`. Some registries disable catalog listing; in that case search may be unavailable, but users can still enter the repository path and tag manually.

Deleting a registry also deletes all credentials that belong to it. Confirm that deployment targets, build jobs, or runtime image pulls no longer depend on those credentials before deleting.

Automated builds need push credentials. Existing-image deployments mainly need the runtime cluster to pull the target image.

When creating a release, the platform first reads tags live from the target registry and repository stored on the deployment target. If the registry API is unavailable, credentials are insufficient, or the repository does not allow tag listing, the release dialog falls back to saved successful build records. Saved build records only prove that an image was built and pushed at that time; they do not guarantee the upstream registry still keeps that tag. If registry cleanup is enabled, confirm that images referenced by released versions are retained before releasing or rolling back.

Deployment targets support Dockerfile Build Args. Users can enter one Dockerfile `ARG` per line as `KEY=value`; the platform snapshots the current config into the BuildRun and passes the values to BuildKit. Build Args support the same build-time templates as image tags: `${{ github.sha }}`, `${{ github.ref_name }}`, `${{ github.ref_type }}`, `${{ github.ref }}`, and `{short_sha}`. Build Args are build parameters and appear in build records, so do not use them for secrets. Put sensitive values in project-space build secret variables.

Build environment variables and secrets support four levels. The platform merges them in `global -> project space -> application -> deployment target` order, and a later level replaces an earlier value with the same key:

- Platform administrators maintain global defaults in Site Settings. They are useful for shared package mirrors or build-tool defaults.
- Project-space values continue to use Build Variables in the project workspace and may combine multiple variable sets.
- Application values are edited on the application's Builds page and apply to every deployment target in that application.
- Deployment-target values are edited in the target's build section and are intended for environment-specific settings.

The merged variables and secret references are snapshotted when a BuildRun is created. Retrying a build uses the original snapshot, so later configuration changes do not alter an existing run. Public values are stored with the build record; secrets remain encrypted references, the API returns only their presence, and the Worker resolves plaintext only while executing the build.

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

If the API or worker runs in a container, kubeconfig server addresses must be reachable from that container. Avoid host-only `127.0.0.1`. The platform accepts only HTTPS API servers with inline CA, client certificate/private key, or token data. It rejects `exec` credential plugins, `auth-provider`, `tokenFile`, `proxy-url`, and local certificate file paths. Run `kubectl config view --raw --minify --flatten` before saving so platform processes never execute external commands or read host files.

Runtime clusters also host Kubernetes build Jobs. The small-team default allows 4 concurrent build Jobs per runtime cluster and 2 concurrent builds per project space. Extra builds stay queued and retry automatically instead of being marked failed immediately.

## Personal tokens

Personal tokens are used by scripts, CI, or external automation to call the platform API. The plain token is shown only once after creation, and the backend stores only a hash. Revoked tokens stop working immediately and are hidden from the list.

Tokens can include multiple scopes. The scope catalog is served by the backend and periodically synchronized by the frontend, so future scope changes do not require hardcoded page updates. Regular users can create read scopes and explicit automation trigger scopes, such as reading project spaces, reading deployments, triggering builds, and creating releases. Platform administrators can create higher-risk scopes such as write, delete, Web Console, secret value access, user management, and site configuration scopes.

Prefer least privilege. CI that only triggers builds should use `build:trigger`; automation that only creates releases should use `deployment:release`; log readers should add `build:read` or `deployment:read` only when needed. Avoid granting unnecessary write or management scopes to long-lived tokens.

## Secrets

Secrets, tokens, and registry credentials are not echoed back. When editing, an empty value means "keep the existing value". Enter a new value only when replacing it.

The console uses a muted `****** (Set)` placeholder when a write-only field already has a value. It is not the stored secret and is never submitted as the form value; entering a replacement simply replaces the placeholder visually.

Registry credentials have two independent controls: `usage` selects pull, push, or both, while `scope` selects personal, multiple project spaces, or global platform use. A credential cannot exceed its registry scope, and sharing never exposes the stored secret value.
