# Configuration and Connections

This page covers site settings and the information required to connect Git providers, registries, and runtime clusters.

## Site and account settings

Administrators can configure the site title, logo, sign-in text, default language, theme, and interface style. Users can override the theme, light or dark mode, and interface style in personal settings. Personal settings take precedence.

Global settings place the save action for the current tab in the tab action area. The button is enabled after a change is made; switching tabs does not save changes automatically.

**Registration and email** controls email and OIDC registration independently. Email registration requires SMTP. The SMTP password is not shown after it is saved; leave it empty when editing other settings to keep the existing value.

## Security policy

Production responses include baseline browser protections. Enable HSTS only when the site is always available over HTTPS, because browsers may otherwise refuse later HTTP access to the domain.

The platform can require step-up MFA for sensitive operations after sign-in; it does not currently request a second factor at every sign-in. Before enabling it globally, ensure that at least one active platform administrator has enrolled an offline TOTP authenticator.

When connecting an authenticator, scan the QR code with a password manager or authenticator that supports verification codes. On iPhone or iPad, use the Camera app to add it to Passwords. An enabled authenticator is never overwritten in place: to connect a new one, complete step-up verification, disable two-step verification, and then enable it again. Disabling invalidates the old authenticator, recovery codes, and existing step-up state.

After setup, Luna DevOps generates 10 recovery codes formatted like `ABCD-EFGH-JKLM-NPQR`. They are shown only once and each can be used once; regenerating them invalidates every old code. Use an unused recovery code if the authenticator is unavailable. If both the authenticator and recovery codes are lost, contact another platform administrator to reset MFA.

Sign-in and step-up verification fields support standard browser password and one-time-code autofill semantics. To use a TOTP stored in a password manager, make sure the matching login item includes the current site address, the extension is unlocked, and inline autofill is enabled. If no suggestion appears, focus the verification-code field and select the code from the password manager.

After creating an account, initializing an administrator, or changing a sign-in password, the browser can offer to save or update the matching credential. When an administrator sets or resets another user's password, the prompt is associated with the target user's email; verify the selected account before saving. Registry credentials, Git access tokens, OIDC client secrets, SMTP passwords, and AI API keys are marked as non-login credentials to avoid filling or overwriting the site password.

## Git providers

Git providers connect GitHub, Gitea, or GitLab for repository search, webhooks, and builds.

1. Create a provider and enter its type and service URL.
2. Configure OAuth or access credentials.
3. Test the connection before binding repositories to projects or applications.

Credentials can be limited to a user, selected projects, or the whole platform. Deleting a provider also deletes its credentials, so first confirm that repository bindings and builds no longer depend on it.

To validate the runtime path first, skip Git and deploy an existing image.

## Registries

Registries store build output and provide images to runtime clusters. Luna DevOps supports Harbor, Gitea Registry, DockerHub, and generic OCI registries.

Choose whether each credential can pull, push, or both, and limit who can use it. Deleting a registry also deletes its credentials; first verify that builds and releases no longer depend on it.

Automated builds require push access. Before starting or retrying a source build, the target registry must have at least one credential visible to the current user or project with **push** or **push-pull** usage. A registry address or pull-only credential cannot receive build output. Deploying existing images requires the cluster to have pull access. Some registries disable repository or tag listing; search may then be unavailable, but you can still enter a full image reference.

Build variables can be set globally, per project, per application, or per deployment target. A more specific value overrides a broader one. Build arguments and plain variables appear in build records and must not contain secrets; use secret variables instead.

## Runtime clusters

A runtime cluster is a release target. Provide a kubeconfig that the platform service can reach, then test the connection.

When the API or worker runs in a container, do not use a kubeconfig address such as `127.0.0.1` that is only reachable from the host. Run `kubectl config view --raw --minify --flatten` and save the output. The platform accepts HTTPS API servers and inline authentication material, not authentication methods that execute local commands or read local files.

The kubeconfig field uses a syntax-highlighted YAML editor. If the enhanced editor cannot load in the browser, the form falls back to a basic text input and keeps the current content, so you can continue editing, validation, and saving.

Cluster settings also include domain suffixes, the external protocol, Gateway, and TLS. Choose a TLS mode based on the traffic path:

- A CDN or upstream proxy terminates HTTPS: choose **TLS terminated by upstream proxy**.
- The cluster Gateway uses an existing certificate: choose **TLS terminated by Gateway** and configure the TLS Secret.
- cert-manager issues certificates: configure an existing Issuer or ClusterIssuer and ensure its challenge is reachable.

The platform does not create ACME accounts or DNS provider credentials. Before changing cluster, Gateway, or certificate settings, confirm the affected routes and recovery plan.

Project, application, and deployment-stage identifiers cannot be changed after creation. A failed application deletion may temporarily prevent identifier reuse. Persistent data is managed independently through project volumes: deleting an application only unbinds its mounts and does not delete the project volume. To reuse the data, select that volume again in a deployment on the same project and runtime cluster; a same-named resource never takes ownership automatically.

## Personal tokens and secrets

Personal tokens are for scripts, CI, and external automation. Their plaintext is shown only once. Store it safely, grant only the required scopes, and revoke it when no longer needed.

Secrets, tokens, and registry credentials are never shown in plaintext. Leaving a secret field empty while editing keeps its existing value; enter a value only to replace it.
