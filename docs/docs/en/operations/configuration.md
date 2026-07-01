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

Registry credentials can define an image repository template and an image tag template. They are used only to seed the default push location when a deployment target is created. After the deployment target is saved, the repository and tag are stored as a snapshot and no longer follow credential template changes. For example, repository template `devopsns/{project}-{app}-{stage}` plus tag template `{projectSlug}-{appSlug}-{stage}` seeds image refs like `devopsns/blog-api-prod:blog-api-prod`.

Repository and tag templates both render only static values known when the deployment target is created: `{registryNamespace}`, `{project}`, `{projectSlug}`, `{app}`, `{appSlug}`, `{stage}`, and `{target}`. If the tag template uses build-time variables such as `{commit}` or `{branch}`, the deployment target default falls back to `latest` so future builds are not implicitly rewritten by credential templates.

## Runtime clusters

Runtime clusters are release targets. The platform turns Releases into Kubernetes resources, then shows status, logs, and diagnostics.

Runtime clusters also own access-route default domain suffixes, external access schemes, external access ports, and Gateway API defaults. Access routes use the deployment target's cluster to generate default domains, expand short host prefixes, and return console access links, so multiple clusters can use different GatewayClasses, shared Gateways, or root domains.

Gateway config is split into external display and internal cluster layers:

- The external access scheme and external access port only affect generated access URLs. HTTP `80` and HTTPS `443` are omitted from URLs; non-standard ports are shown as `:port`. They do not change Kubernetes Gateway listeners or request certificates.
- Gateway listener names and ports are the internal Gateway/Controller settings that receive traffic inside the cluster. The defaults are `web:8080` and `websecure:8443`. Project users do not choose ports or listeners.

The external TLS mode decides which listener access routes bind to by default. "TLS terminates at Gateway" binds routes to the HTTPS listener, such as `websecure`; "TLS terminates upstream" binds routes to the HTTP listener, such as `web`, because traffic entering the Gateway is already cleartext HTTP. The HTTPS listener is always emitted as an HTTPS/TLS entrypoint to match Traefik `websecure` entryPoints with TLS enabled.

If an outer Nginx/CDN/load balancer already owns host ports `80/443`, point it to the cluster Gateway's internal ports: upstream TLS termination should forward to the HTTP listener, such as `8080`; Gateway TLS termination should forward to the HTTPS listener, such as `8443`. The platform still renders access URLs from the runtime cluster's external access scheme and external access port settings.

The cluster resource page lists platform-managed namespaces, workloads, services, configs, secrets, and storage with server-side pagination. Only resources visible to the current user are counted in the page total. The workload tab uses Deployment rows as the top level; expanding a Deployment shows its Pods as child rows, and those Pod rows are not counted by pagination.

If the API or worker runs in a container, kubeconfig server addresses must be reachable from that container. Avoid host-only `127.0.0.1`.

Runtime clusters also host Kubernetes build Jobs. The small-team default allows 4 concurrent build Jobs per runtime cluster and 2 concurrent builds per project space. Extra builds stay queued and retry automatically instead of being marked failed immediately.

## Secrets

Secrets, tokens, and registry credentials are not echoed back. When editing, an empty value means "keep the existing value". Enter a new value only when replacing it.
