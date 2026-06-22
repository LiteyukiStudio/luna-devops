# Settings and Connections

Settings fall into two groups: public console settings and backend connections to external systems.

## Public settings

Public settings affect what users see:

- Site title.
- Logo and favicon.
- Sign-in subtitle.
- Theme and language preferences.

These can be shown to the frontend, but should never contain tokens, passwords, or internal-only addresses.

## Git providers

Git providers connect GitHub or Gitea. After setup, users can bind repositories, receive webhooks, and trigger builds by branch or tag.

Deleting a Git provider also deletes all Git credentials that belong to it. Confirm that repository bindings and build flows no longer depend on those credentials before deleting.

If you only want to test deployment first, skip Git providers and use an existing image.

## Registries

Registries store or provide images. Common choices include Harbor, Gitea Registry, and DockerHub.

Deleting a registry also deletes all credentials that belong to it. Confirm that deployment targets, build jobs, or runtime image pulls no longer depend on those credentials before deleting.

Automated builds need push credentials. Existing-image deployments mainly need the runtime cluster to pull the target image.

## Runtime clusters

Runtime clusters are release targets. The platform turns Releases into Kubernetes resources, then shows status, logs, and diagnostics.

The cluster resource page lists platform-managed namespaces, workloads, services, configs, secrets, and storage with server-side pagination. Only resources visible to the current user are counted in the page total.

If the API or worker runs in a container, kubeconfig server addresses must be reachable from that container. Avoid host-only `127.0.0.1`.

Runtime clusters also host Kubernetes build Jobs. The small-team default allows 4 concurrent build Jobs per runtime cluster and 2 concurrent builds per project space. Extra builds stay queued and retry automatically instead of being marked failed immediately.

## Secrets

Secrets, tokens, and registry credentials are not echoed back. When editing, an empty value means "keep the existing value". Enter a new value only when replacing it.
