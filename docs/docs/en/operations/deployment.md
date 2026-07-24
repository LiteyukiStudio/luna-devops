# How the Platform Fits Together

Luna DevOps connects code, images, clusters, and routes into one delivery path. On the first day, focus on this flow:

```text
Project space -> Application -> Deployment target -> Build -> Release -> Route
```

For a click-through walkthrough, see [Deploy a Web Project](/en/operations/deploy-web-project). It uses `snowykami/neo-blog` as the example and goes from project space creation to public access.

## Start with the flow

### 1. Project space

A project space is the boundary for teams and resources. Members, applications, deployment targets, build records, releases, and routes all belong to a project space.

Common patterns:

- One product per project space.
- One small team per project space.
- One customer or demo environment per project space.

When adding members, Owner/Admin users search existing platform users and select from suggestions. Free-form email text is not submitted as a member by itself.

### 2. Application

An application represents one deployable service. A single repository can map to multiple applications, such as API, Web, and Worker in a monorepo.

The application stores basic service information only. Build source, image, environment variables, resource size, and release policy live in deployment targets.

The application detail page includes a live topology view. It shows the `Gateway -> HTTPRoute -> Service -> Deployment/StatefulSet -> Pod` delivery path by default; enable dependencies to include HPA, ConfigMap, Secret, and PVC resources. The topology is not stored in the database. Every open or refresh recomputes it from the runtime cluster, so manual out-of-band resource deletion is reflected as well. Secret nodes expose only resource names and status, never Secret contents.

A project space can also describe logical relationships between applications. Regular members see the project topology only after at least one relationship exists. Owners and Admins get a small advanced entry point for creating the first relationship, so teams that do not need this feature are not asked to configure it.

There are two relationship modes:

- A **service binding** affects the source deployment target. The platform derives a stable in-cluster address from the target Kubernetes Service and injects environment variables on the source target's next release.
- A **display-only relationship** records calls, reads/writes, publishing, or consumption without changing deployment configuration or creating Kubernetes resources.

The first service-binding version requires both targets to be in the same project space and runtime cluster, and the source and target deployment targets must be selected explicitly. Passwords and tokens are never embedded in the generated address; keep database credentials and other sensitive values in project-space Secrets. After saving, you can stay on the topology page or open the source target's release page to select an image and complete the existing confirmation flow. Existing Pods do not change until a new release is created.

Diagnostics inspect the target Service, selected port, EndpointSlices, and NetworkPolicies that may affect connectivity. They only read Kubernetes metadata, do not dial the application port, and never read Secret contents. An application or deployment target with active incoming service bindings cannot be deleted until those bindings are disabled or removed.

### 3. Deployment target

A deployment target decides how an application is built, how it runs, and where it is released.

For the first target, focus on:

- Build source: build from a repository or use an existing image.
- Stage: development, test, staging, or production.
- Registry: where build output is pushed.
- Service ports: multiple ports are supported, and the first port is the default.
- Runtime configuration: replicas, CPU, memory, project-level shared configuration, and deployment-level overrides.
- Build policy: timeout and whether a successful build should auto release.

The deployment target form follows a clear order: Basic deployment, Build settings, Runtime configuration, Release policy, Deployment hooks, Runtime data, and Advanced Kubernetes configuration. Build settings appear only for repository sources. Runtime resources and injected configuration are managed together immediately after build settings, while longer guidance is available from the help control beside each section title.

For a new service, it is often easier to create the first Release from an existing image. After the Pod and route are healthy, connect a Git provider and enable automated builds.

### 4. Build and release

Builds create images. Releases deploy images to runtime clusters.

Every new Release updates the Kubernetes Pod Template release fingerprint, so a rollout happens even when the image tag stays the same. By default, config-only changes restart Pods without forcing an image pull. When a Release comes from a new build artifact and the tag stays the same, the platform temporarily uses `imagePullPolicy: Always`.

If remote image content changed but the tag did not, choose “Pull latest image and deploy” from the deployment target actions menu.

## Advanced target settings

Most services can keep the defaults. Expand advanced settings only when the image or runtime environment needs them.

### Kubernetes runtime options

| Scenario | What to adjust |
| --- | --- |
| Container should not use the default entrypoint | `command`, `args`, `imagePullPolicy` |
| Health checks are needed | readiness, liveness, startup probes |
| Stateful behavior is needed | switch from Deployment to StatefulSet |
| Startup or shutdown hooks are needed | Kubernetes Lifecycle `postStart` / `preStop` |
| Helper containers are needed | initContainers and sidecars |
| Resource caps are needed | CPU / memory limits |
| Auto scaling is needed | HPA CPU/memory targets and behavior |
| Fixed UID/GID or stronger isolation is needed | securityContext, read-only root filesystem, capabilities |
| Specific scheduling is needed | nodeSelector, tolerations, affinity, topologySpreadConstraints |
| Service or storage needs tuning | Service type, annotations, PVC storageClass/accessMode/volumeMode |

Complex fields use native Kubernetes JSON, such as Probe, Toleration, Affinity, and TopologySpreadConstraint. Simple key-value fields accept either a JSON object or `KEY=VALUE` lines.

### Storage notes

PVC `storageClassName` and `accessMode` are written only when the data volume is first created. Existing PVCs are resized only; the platform does not migrate or recreate storage automatically.

`emptyDir` data is destroyed with the Pod and is not exported by the platform. Existing PVCs can be exported, but the platform does not manage their capacity or lifecycle.

### Config, variables, and hooks

When build variables or runtime config sets are deleted, the platform removes those references from deployment targets that still point to them, so targets do not keep stale configuration IDs.

When a deployment target attaches a project-space runtime config set, it can use either mode:

- Live reference: the next release reads the latest shared config.
- Snapshot: the target freezes the current config when it is saved.

Project-space hooks are reusable script definitions. They only run after a deployment target binds them to phases such as before build, after build, before image push, before deploy, or after deploy. Pre-deploy hooks fit database migrations, seed commands, or one-shot repair tasks.

Hooks run as Kubernetes Jobs. The platform stores hook records and logs; in-cluster Jobs/Pods are kept only briefly for troubleshooting. Successful hooks are cleaned up after 5 minutes by default, and failed hooks after 24 hours.

## Web Console and data export

### Web Console

The project-space Web Console master switch is enabled by default. Project Owners/Admins can disable it for the whole space. A deployment target can inherit the project setting or disable itself, but it cannot override a disabled project switch.

This switch only controls terminal availability. It does not relax roles or MFA:

- Application release terminal: project Owner, Admin, and Developer.
- Cluster resource Pod terminal: platform administrators only.

Runtime-command audits store command summary, length, container, and exit code, not the original command body. Interactive-terminal audits store connection target and result, not terminal input or output.

### Data export

Deployment-target data export requires a browser cookie session and project Owner/Admin role. A personal access token is rejected even if it has a data-export scope.

Before download, the platform issues a 60-second one-time ticket bound to the current user, session, project, application, and deployment target. Production replicas store only the ticket hash in Redis; when Redis is unavailable, export fails closed.

Exports include platform-managed or existing PVCs only. `emptyDir` is never exported.

## Routes

Routes connect domain, path, TLS, and backend service. After creation, the platform shows apply status and checks so you can verify the service is reachable.

### Domain and port

Domain suffixes come from the deployment target's runtime cluster. Administrators can maintain multiple suffixes on a cluster, and each route selects one. Short host prefixes and generated default domains use the selected suffix.

When a route is created or enabled, the platform first checks that the target Kubernetes Service and port exist. If the Service was deleted manually, redeploy the deployment target to restore runtime resources.

### Choosing TLS

| Scenario | Recommended mode |
| --- | --- |
| CDN or outer reverse proxy already terminates HTTPS | Set external access scheme to `https` and choose upstream TLS termination |
| Cluster Gateway terminates HTTPS | Choose Gateway termination and configure an existing Kubernetes TLS Secret |
| cert-manager HTTP-01 is used | Choose HTTP Challenge certificate mode and prepare an Issuer/ClusterIssuer first |
| DNS-01 wildcard certificate is used | Configure wildcard certificate Secret on the runtime cluster and reuse it for the suffix |

The platform only references the runtime cluster's `Issuer` or `ClusterIssuer`; it does not create an ACME account. The default name `letsencrypt-http01` is just an Issuer resource name. CA, email, account Secret, and solver settings come from that Issuer.

### Gateway API

Routes are backed by Kubernetes Gateway API. A runtime cluster owns one platform-managed `Gateway`; each route creates an `HTTPRoute` in the project namespace and forwards to the deployment target `Service`.

Install Gateway API CRDs before enabling routes. Traefik clusters also need `--providers.kubernetesGateway`.

### Forwarded headers and real scheme

For a chain like:

```text
CDN HTTPS -> Nginx HTTP -> Traefik HTTP -> Pod
```

Prefer configuring Traefik entryPoint `forwardedHeaders.trustedIPs` to trust the upstream proxy and forward `X-Forwarded-Proto=https`.

Apps such as Logto/OIDC providers may generate the wrong issuer or redirect URL if the backend sees `http`. When needed, choose upstream TLS + overwrite on the runtime cluster so the platform injects `X-Forwarded-Proto=https` and `X-Forwarded-Port=443` through HTTPRoute RequestHeaderModifier.
